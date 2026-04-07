package main

import (
	"context"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/elastic/go-elasticsearch/v9"
	amqp "github.com/rabbitmq/amqp091-go"

	"integration-project-ehb/controlroom/internal/cr_rabbitmq"
	"integration-project-ehb/controlroom/internal/heartbeat"
	"integration-project-ehb/controlroom/internal/statuscheck"
	"integration-project-ehb/controlroom/internal/userobject"
)

func main() {
	// Elasticsearch client
	esClient, err := elasticsearch.NewDefaultClient()
	if err != nil {
		log.Fatalf("elasticsearch client: %v", err)
	}
	res, err := esClient.Info()
	if err != nil {
		log.Fatalf("elasticsearch connect: %v", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(res.Body)
	log.Println("Connected to Elasticsearch")

	// RabbitMQ connection
	conn, err := amqp.Dial(os.Getenv("RABBITMQ_URL"))
	if err != nil {
		log.Fatalf("rabbitmq dial: %v", err)
	}
	defer func(conn *amqp.Connection) {
		err := conn.Close()
		if err != nil {

		}
	}(conn)
	log.Println("Connected to RabbitMQ")

	// DLQ channel (shared)
	dlqCh, err := conn.Channel()
	if err != nil {
		log.Fatalf("dlq channel: %v", err)
	}
	defer func(dlqCh *amqp.Channel) {
		err := dlqCh.Close()
		if err != nil {

		}
	}(dlqCh)

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Heartbeat consumer
	hbCh, err := conn.Channel()
	if err != nil {
		log.Fatalf("heartbeat channel: %v", err)
	}
	defer func(hbCh *amqp.Channel) {
		err := hbCh.Close()
		if err != nil {

		}
	}(hbCh)

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
		Process: heartbeat.NewHeartbeatProcessor(esClient),
	}

	if err := cr_rabbitmq.SetupDLQ(hbCfg.DLQCh, hbCfg.DLQName); err != nil {
		log.Fatalf("heartbeat dlq setup: %v", err)
	}

	if err != nil {
		log.Fatalf("heartbeat setup: %v", err)
	}

	// NOTE(nasr): prefetch count 18 for reasonable throughput without hoarding memory
	err = hbCh.Qos(18, 0, false)
	if err != nil {
		return
	}

	go cr_rabbitmq.Consume(hbCfg, hbMsgs, ctx)
	log.Println("Heartbeat consumer started")

	// User consumer
	userCh, err := conn.Channel()
	if err != nil {
		log.Fatalf("user channel: %v", err)
	}
	defer func(userCh *amqp.Channel) {
		err := userCh.Close()
		if err != nil {

		}
	}(userCh)

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
		log.Fatalf("user setup: %v", err)
	}

	userCfg := &cr_rabbitmq.ConsumerConfig{
		DLQCh:   dlqCh,
		DLQName: "user_dlq",
		Process: userobject.NewUserProcessor(esClient),
	}
	if err := cr_rabbitmq.SetupDLQ(userCfg.DLQCh, userCfg.DLQName); err != nil {
		log.Fatalf("user dlq setup: %v", err)
	}

	// NOTE(nasr): prefetch count 10 for higher throughput, autoack disabled per consumer
	err = userCh.Qos(10, 0, false)
	if err != nil {
		return
	}

	go cr_rabbitmq.Consume(userCfg, userMsgs, ctx)
	log.Println("User consumer started")

	// StatusCheck consumer
	scCh, err := conn.Channel()
	if err != nil {
		log.Fatalf("statuscheck channel: %v", err)
	}
	defer func(scCh *amqp.Channel) {
		err := scCh.Close()
		if err != nil {

		}
	}(scCh)

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
		log.Fatalf("statuscheck setup: %v", err)
	}

	scCfg := &cr_rabbitmq.ConsumerConfig{
		DLQCh:   dlqCh,
		DLQName: "statuscheck_dlq",
		Process: statuscheck.NewStatusCheckProcessor(esClient),
	}
	if err := cr_rabbitmq.SetupDLQ(scCfg.DLQCh, scCfg.DLQName); err != nil {
		log.Fatalf("statuscheck dlq setup: %v", err)
	}

	err = scCh.Qos(5, 0, false)
	if err != nil {
		return
	}

	go cr_rabbitmq.Consume(scCfg, scMsgs, ctx)
	log.Println("StatusCheck consumer started")

	// TODO(nasr): add logging stuff in the future

	// ---------------------- shutdown stuff ----------------------
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	log.Println("Shutdown signal received, draining queues...")
	cancel()

	// Give consumers time to ack pending messages
	// TODO(nasr): implement proper drain with timeout
	// time.Sleep(2 * time.Second)
}
