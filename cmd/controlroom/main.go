package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	elasticsearch "github.com/elastic/go-elasticsearch/v9"
	amqp "github.com/rabbitmq/amqp091-go"

	"integration-project-ehb/controlroom/internal/cr_rabbitmq"
	"integration-project-ehb/controlroom/internal/heartbeat"
	"integration-project-ehb/controlroom/internal/statuscheck"
	"integration-project-ehb/controlroom/internal/userobject"
	"integration-project-ehb/controlroom/pkg/logger"
)

func main() {
	// Basic logger for startup sequence (no ES yet)
	log := logger.New(os.Stdout, "controlroom")

	// Elasticsearch client
	esClient, err := elasticsearch.NewDefaultClient()
	if err != nil {
		log.Fatal("elasticsearch client", err)
	}
	res, err := esClient.Info()
	if err != nil {
		log.Fatal("elasticsearch connect", err)
	}
	defer res.Body.Close()

	// Upgrade to ES-backed logger now that connection is confirmed
	log = logger.NewWithElastic(os.Stdout, "controlroom", esClient, "controlroom-logs")
	defer log.Flush()
	log.Info("connected to elasticsearch")

	// RabbitMQ connection
	conn, err := amqp.Dial(os.Getenv("RABBITMQ_URL"))
	if err != nil {
		log.Fatal("rabbitmq dial", err)
	}
	defer conn.Close()
	log.Info("connected to rabbitmq")

	// DLQ channel (shared)
	dlqCh, err := conn.Channel()
	if err != nil {
		log.Fatal("dlq channel", err)
	}
	defer dlqCh.Close()

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Heartbeat consumer
	hbCh, err := conn.Channel()
	if err != nil {
		log.Fatal("heartbeat channel", err)
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

	hbCfg := &cr_rabbitmq.ConsumerConfig{
		DLQCh:   dlqCh,
		DLQName: "heartbeat_dlq",
		Process: heartbeat.NewHeartbeatProcessor(esClient, log),
		Log:     log,
	}

	if err := cr_rabbitmq.SetupDLQ(hbCfg.DLQCh, hbCfg.DLQName); err != nil {
		log.Fatal("heartbeat dlq setup", err)
	}

	if err != nil {
		log.Fatal("heartbeat setup", err)
	}

	// NOTE(nasr): prefetch count 18 for reasonable throughput without hoarding memory
	hbCh.Qos(18, 0, false)

	go cr_rabbitmq.Consume(hbCfg, hbMsgs, ctx)
	log.Info("heartbeat consumer started")

	// User consumer
	userCh, err := conn.Channel()
	if err != nil {
		log.Fatal("user channel", err)
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
		log.Fatal("user setup", err)
	}

	userCfg := &cr_rabbitmq.ConsumerConfig{
		DLQCh:   dlqCh,
		DLQName: "user_dlq",
		Process: userobject.NewUserProcessor(esClient, log),
		Log:     log,
	}
	if err := cr_rabbitmq.SetupDLQ(userCfg.DLQCh, userCfg.DLQName); err != nil {
		log.Fatal("user dlq setup", err)
	}

	// NOTE(nasr): prefetch count 10 for higher throughput, autoack disabled per consumer
	userCh.Qos(10, 0, false)

	go cr_rabbitmq.Consume(userCfg, userMsgs, ctx)
	log.Info("user consumer started")

	// StatusCheck consumer
	scCh, err := conn.Channel()
	if err != nil {
		log.Fatal("statuscheck channel", err)
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
		log.Fatal("statuscheck setup", err)
	}

	scCfg := &cr_rabbitmq.ConsumerConfig{
		DLQCh:   dlqCh,
		DLQName: "statuscheck_dlq",
		Process: statuscheck.NewStatusCheckProcessor(esClient, log),
		Log:     log,
	}
	if err := cr_rabbitmq.SetupDLQ(scCfg.DLQCh, scCfg.DLQName); err != nil {
		log.Fatal("statuscheck dlq setup", err)
	}

	scCh.Qos(5, 0, false)

	go cr_rabbitmq.Consume(scCfg, scMsgs, ctx)
	log.Info("statuscheck consumer started")

	// ---------------------- shutdown stuff ----------------------
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	log.Info("shutdown signal received")
	cancel()

	// Give consumers time to ack pending messages
	// TODO(nasr): implement proper drain with timeout
	// time.Sleep(2 * time.Second)
}
