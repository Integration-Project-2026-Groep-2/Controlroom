package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	amqp "github.com/rabbitmq/amqp091-go"

	"integration-project-ehb/controlroom/internal/company"
	"integration-project-ehb/controlroom/internal/cr_logger"
	"integration-project-ehb/controlroom/internal/cr_rabbitmq"
	"integration-project-ehb/controlroom/internal/heartbeat"
	"integration-project-ehb/controlroom/internal/statuscheck"
	"integration-project-ehb/controlroom/internal/user"
	"integration-project-ehb/controlroom/pkg/logger"
	"integration-project-ehb/controlroom/pkg/watchdog"
)

func main() {

	cfg := elasticsearch.Config{
		Addresses: []string{os.Getenv("ELASTICSEARCH_URL")},
		Username:  os.Getenv("CONTROLROOM_ES_USER"),
		Password:  os.Getenv("CONTROLROOM_ES_PASS"),
	}

	if err := logger.Init(os.Getenv("ELASTICSEARCH_URL"), "controlroom-logs", os.Stdout, 4); err != nil {
		fmt.Fprintf(os.Stderr, "logger init: %v\n", err)
		os.Exit(1)
	}

	defer logger.Shutdown()

	client, err := elasticsearch.NewClient(cfg)

	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("elasticsearch client config: %v", err)))
	}

	res, err := client.Info()
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("elasticsearch connect: %v", err)))
	}
	res.Body.Close()

	logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, "connected to elasticsearch"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, "shutdown signal received, draining queues..."))
		cancel()
	}()

	const (
		initialBackoff = 1 * time.Second
		maxBackoff     = 60 * time.Second
		healthyAfter   = 10 * time.Second
	)

	backoff := initialBackoff

	if watchdog.WDWebhook == "" {
		logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, "CRITICAL: TEAMS_WEBHOOK_URL is niet ingesteld in de environment!"))
	}

	for _, svc := range watchdog.WDServices {
		watchdog.WDServiceState[svc] = true
	}

	go watchdog.ProcessAlertQueue()

	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				watchdog.CheckHeartbeats(client)
			case <-ctx.Done():
				logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, "Watchdog started! Checking heartbeats every 60 seconds..."))
				return
			}
		}
	}()

	for {
		start := time.Now()

		err := startSession(ctx, client)

		if errors.Is(err, context.Canceled) {
			return
		}
		if time.Since(start) > healthyAfter {
			backoff = initialBackoff
		}

		logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, fmt.Sprintf("rabbit session ended, redialing in %s: %v", backoff, err)))

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

func startSession(ctx context.Context, client *elasticsearch.Client) error {
	conn, err := amqp.Dial(os.Getenv("RABBITMQ_URL"))
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()
	logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, "connected to RabbitMQ"))

	closeCh := conn.NotifyClose(make(chan *amqp.Error, 1))

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
		Name:    "heartbeat.queue",
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
		DLQName: "heartbeat.dlq",
		Process: heartbeat.NewHeartbeatProcessor(client),
	}

	if err := cr_rabbitmq.SetupDLQ(hbCfg.DLQCh, hbCfg.DLQName); err != nil {
		return fmt.Errorf("heartbeat dlq setup: %w", err)
	}
	if err := hbCh.Qos(18, 0, false); err != nil {
		return fmt.Errorf("heartbeat qos: %w", err)
	}
	go cr_rabbitmq.Consume(hbCfg, hbMsgs, ctx)
	logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, "heartbeat consumer started"))

	// User consumer
	userCh, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("user channel: %w", err)
	}
	defer userCh.Close()

	userMsgs, err := cr_rabbitmq.SetupQueue(userCh,

		cr_rabbitmq.ExchangeInfo{
			Name:    "contact.topic",
			Kind:    "topic",
			Durable: true,
		},

		cr_rabbitmq.QueueInfo{
			Name:    "crm.user.confirmed",
			Durable: true,
		},

		cr_rabbitmq.BindingInfo{
			Key: "crm.user.confirmed",
		},
	)

	if err != nil {
		return fmt.Errorf("user setup: %w", err)
	}

	userCfg := &cr_rabbitmq.ConsumerConfig{
		DLQCh:   dlqCh,
		DLQName: "user.dlq",
		Process: user.NewUserProcessor(client),
	}
	if err := cr_rabbitmq.SetupDLQ(userCfg.DLQCh, userCfg.DLQName); err != nil {
		return fmt.Errorf("user dlq setup: %w", err)
	}
	if err := userCh.Qos(10, 0, false); err != nil {
		return fmt.Errorf("user qos: %w", err)
	}
	go cr_rabbitmq.Consume(userCfg, userMsgs, ctx)
	logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, "user consumer started"))

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
		Name:    "statuscheck.queue",
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
		DLQName: "statuscheck.dlq",
		Process: statuscheck.NewStatusCheckProcessor(client),
	}
	if err := cr_rabbitmq.SetupDLQ(scCfg.DLQCh, scCfg.DLQName); err != nil {
		return fmt.Errorf("statuscheck dlq setup: %w", err)
	}
	if err := scCh.Qos(5, 0, false); err != nil {
		return fmt.Errorf("statuscheck qos: %w", err)
	}
	go cr_rabbitmq.Consume(scCfg, scMsgs, ctx)
	logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, "statuscheck consumer started"))

	// Company consumer
	companyCh, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("company channel: %w", err)
	}

	defer companyCh.Close()

	companyMsgs, err := cr_rabbitmq.SetupQueue(companyCh,
		cr_rabbitmq.ExchangeInfo{Name: "contact.topic", Kind: "topic", Durable: true},
		cr_rabbitmq.QueueInfo{Name: "crm.company.confirmed", Durable: true},
		cr_rabbitmq.BindingInfo{Key: "crm.company.confirmed"},
	)

	if err != nil {
		return fmt.Errorf("company setup: %w", err)
	}

	companyCfg := &cr_rabbitmq.ConsumerConfig{
		DLQCh:   dlqCh,
		DLQName: "company.dlq",
		Process: company.NewCompanyProcessor(client),
	}

	if err := cr_rabbitmq.SetupDLQ(companyCfg.DLQCh, companyCfg.DLQName); err != nil {
		return fmt.Errorf("company dlq setup: %w", err)
	}

	if err := companyCh.Qos(10, 0, false); err != nil {
		return fmt.Errorf("company qos: %w", err)
	}

	go cr_rabbitmq.Consume(companyCfg, companyMsgs, ctx)
	logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, "company consumer started"))

	logCh, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("log channel: %w", err)
	}

	defer companyCh.Close()

	msgs, err := cr_rabbitmq.SetupQueue(
		logCh,
		cr_rabbitmq.ExchangeInfo{
			Name:       "logs.direct",
			Kind:       "direct",
			Durable:    true,
			AutoDelete: false,
			Internal:   false,
			NoWait:     false,
			Args:       nil,
		},

		cr_rabbitmq.QueueInfo{
			Name:       "logs.queue",
			Durable:    true,
			AutoDelete: false,
			Exclusive:  false,
			NoWait:     false,
			Args:       nil,
		},

		cr_rabbitmq.BindingInfo{
			Key:    "log",
			NoWait: false,
			Args:   nil,
		},
	)
	if err != nil {
		return fmt.Errorf("SetupLogsConsumer: %w", err)
	}

	cfg := &cr_rabbitmq.ConsumerConfig{
		DLQCh:   dlqCh,
		DLQName: "dlq",
		Process: cr_logger.LogMessageProcesser,
	}

	go cr_rabbitmq.Consume(cfg, msgs, ctx)

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
