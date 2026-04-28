package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	amqp "github.com/rabbitmq/amqp091-go"

	"integration-project-ehb/controlroom/internal/company"
	"integration-project-ehb/controlroom/internal/cr_rabbitmq"
	"integration-project-ehb/controlroom/internal/heartbeat"
	"integration-project-ehb/controlroom/internal/statuscheck"
	"integration-project-ehb/controlroom/internal/user"
	"integration-project-ehb/controlroom/pkg/logger"
)

func main() {
	// Elasticsearch client
	cfg := elasticsearch.Config{
		Addresses: []string{os.Getenv("ELASTICSEARCH_URL")},
		Username:  os.Getenv("CONTROLROOM_ES_USER"),
		Password:  os.Getenv("CONTROLROOM_ES_PASS"),
	}

	esClient, err := elasticsearch.NewClient(cfg)
	if err != nil {
		logger.Log("elasticsearch client config", err)
	}

	res, err := esClient.Info()

	if err != nil {
		logger.Log("elasticsearch connect", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(res.Body)

	logger.Log("Connected to Elasticsearch")

	// Vanaf hier gaan alle logs ook naar ES index "controlroom-logs".
	logger.Log(esClient, "controlroom-logs")

	// Context + signal handler for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		internal_logger.Info("Shutdown signal received, draining queues...")
		cancel()
	}()

	// RabbitMQ redial loop. Een sessie leeft zolang de connectie gezond is;
	// bij verlies of setup-fout wachten we met exponential backoff (1s → 60s max)
	// en proberen we opnieuw. De backoff reset zodra een sessie langer dan 10s
	// heeft gedraaid (teken van een succesvolle reconnect).
	const (
		initialBackoff = 1 * time.Second
		maxBackoff     = 60 * time.Second
		healthyAfter   = 10 * time.Second
	)

	backoff := initialBackoff

	for {
		start := time.Now()
		err := startSession(ctx, esClient, internal_logger)
		if errors.Is(err, context.Canceled) {
			return
		}
		if time.Since(start) > healthyAfter {
			backoff = initialBackoff
		}

		internal_logger.Error("rabbit session ended, redialing", err, logger.String("interval", backoff.String()))

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// startSession dials RabbitMQ, declares all four consumers, and blocks
// until the connection drops or ctx is cancelled. Returns an error describing
// why the session ended; the caller decides whether to retry.
func startSession(ctx context.Context, esClient *elasticsearch.Client, internal_logger *logger.Logger) error {
	conn, err := amqp.Dial(os.Getenv("RABBITMQ_URL"))
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()
	internal_logger.Info("Connected to RabbitMQ")

	// Buffered size 1 per amqp091-go convention — otherwise the library's
	// internal sender blocks if we haven't selected yet when the close fires.
	closeCh := conn.NotifyClose(make(chan *amqp.Error, 1))

	// DLQ channel (shared)
	dlqCh, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("dlq channel: %w", err)
	}
	defer dlqCh.Close()

	// Heartbeat consumer
	hbCh, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("heartbeat channel: %w", err)
	}
	defer hbCh.Close()

	hbExchange := cr_rabbitmq.ExchangeInfo{
		Name:    "heartbeat.direct",
		Kind:    "direct",
		Durable: true,
	}
	hbQueue := cr_rabbitmq.QueueInfo{
		Name:    "heartbeat_queue",
		Durable: true,
	}
	hbBinding := cr_rabbitmq.BindingInfo{
		Key: "routing.heartbeat",
	}

	hbMsgs, err := cr_rabbitmq.SetupQueue(hbCh, hbExchange, hbQueue, hbBinding)
	if err != nil {
		return fmt.Errorf("heartbeat setup: %w", err)
	}

	hbCfg := &cr_rabbitmq.ConsumerConfig{
		DLQCh:   dlqCh,
		DLQName: "heartbeat_dlq",
		Process: heartbeat.NewHeartbeatProcessor(esClient),
	}

	if err := cr_rabbitmq.SetupDLQ(hbCfg.DLQCh, hbCfg.DLQName); err != nil {
		return fmt.Errorf("heartbeat dlq setup: %w", err)
	}

	// NOTE(nasr): prefetch count 18 for reasonable throughput without hoarding memory
	if err := hbCh.Qos(18, 0, false); err != nil {
		return fmt.Errorf("heartbeat qos: %w", err)
	}

	go cr_rabbitmq.Consume(hbCfg, hbMsgs, ctx)
	internal_logger.Info("Heartbeat consumer started")

	// User consumer
	userCh, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("user channel: %w", err)
	}
	defer userCh.Close()

	userExchange := cr_rabbitmq.ExchangeInfo{
		Name:    "contact.topic",
		Kind:    "topic",
		Durable: true,
	}

	userQueue := cr_rabbitmq.QueueInfo{
		Name:    "crm.user.confirmed",
		Durable: true,
	}

	userBinding := cr_rabbitmq.BindingInfo{
		Key: "crm.user.confirmed",
	}

	userMsgs, err := cr_rabbitmq.SetupQueue(userCh, userExchange, userQueue, userBinding)
	if err != nil {
		return fmt.Errorf("user setup: %w", err)
	}

	userCfg := &cr_rabbitmq.ConsumerConfig{
		DLQCh:   dlqCh,
		DLQName: "user_dlq",
		Process: user.NewUserProcessor(esClient),
	}
	if err := cr_rabbitmq.SetupDLQ(userCfg.DLQCh, userCfg.DLQName); err != nil {
		return fmt.Errorf("user dlq setup: %w", err)
	}

	// NOTE(nasr): prefetch count 10 for higher throughput, autoack disabled per consumer
	if err := userCh.Qos(10, 0, false); err != nil {
		return fmt.Errorf("user qos: %w", err)
	}

	go cr_rabbitmq.Consume(userCfg, userMsgs, ctx)
	internal_loggger.Info("User consumer started")

	// StatusCheck consumer
	scCh, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("statuscheck channel: %w", err)
	}
	defer scCh.Close()

	scExchange := cr_rabbitmq.ExchangeInfo{
		Name:    "statuscheck.direct",
		Kind:    "direct",
		Durable: true,
	}
	scQueue := cr_rabbitmq.QueueInfo{
		Name:    "statuscheck_queue",
		Durable: true,
	}
	// NOTE(nasr): allows for crm.status.checked, kassa.status.checked, etc.
	scBinding := cr_rabbitmq.BindingInfo{
		Key: "routing.statuscheck",
	}

	scMsgs, err := cr_rabbitmq.SetupQueue(scCh, scExchange, scQueue, scBinding)
	if err != nil {
		return fmt.Errorf("statuscheck setup: %w", err)
	}

	scCfg := &cr_rabbitmq.ConsumerConfig{
		DLQCh:   dlqCh,
		DLQName: "statuscheck_dlq",
		Process: statuscheck.NewStatusCheckProcessor(esClient),
	}
	if err := cr_rabbitmq.SetupDLQ(scCfg.DLQCh, scCfg.DLQName); err != nil {
		return fmt.Errorf("statuscheck dlq setup: %w", err)
	}

	if err := scCh.Qos(5, 0, false); err != nil {
		return fmt.Errorf("statuscheck qos: %w", err)
	}

	go cr_rabbitmq.Consume(scCfg, scMsgs, ctx)
	internal_logger.Info("StatusCheck consumer started")

	// Company consumer
	companyCh, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("company channel: %w", err)
	}
	defer companyCh.Close()

	companyExchange := cr_rabbitmq.ExchangeInfo{
		Name:    "contact.topic",
		Kind:    "topic",
		Durable: true,
	}

	companyQueue := cr_rabbitmq.QueueInfo{
		Name:    "crm.company.confirmed",
		Durable: true,
	}

	companyBinding := cr_rabbitmq.BindingInfo{
		Key: "crm.company.confirmed",
	}

	companyMsgs, err := cr_rabbitmq.SetupQueue(companyCh, companyExchange, companyQueue, companyBinding)
	if err != nil {
		return fmt.Errorf("company setup: %w", err)
	}

	companyCfg := &cr_rabbitmq.ConsumerConfig{
		DLQCh:   dlqCh,
		DLQName: "company_dlq",
		Process: company.NewCompanyProcessor(esClient),
	}
	if err := cr_rabbitmq.SetupDLQ(companyCfg.DLQCh, companyCfg.DLQName); err != nil {
		return fmt.Errorf("company dlq setup: %w", err)
	}

	// NOTE(nasr): verify prefetch count with team
	if err := companyCh.Qos(10, 0, false); err != nil {
		return fmt.Errorf("company qos: %w", err)
	}

	go cr_rabbitmq.Consume(companyCfg, companyMsgs, ctx)
	internal_logger.Info("Company consumer started")

	// Wacht tot de connectie dichtgaat of shutdown wordt geïnitieerd.
	// De Consume-goroutines exiten vanzelf zodra hun msgs channel sluit.
	select {
	case reason := <-closeCh:
		if reason == nil {
			return fmt.Errorf("connection closed")
		}
		return fmt.Errorf("connection lost: %w", reason)
	case <-ctx.Done():
		return ctx.Err()
	}
}
