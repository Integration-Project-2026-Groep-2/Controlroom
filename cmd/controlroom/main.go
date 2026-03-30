package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"integration-project-ehb/controlroom/internal/heartbeat"
	cr_rabbitmq "integration-project-ehb/controlroom/internal/rabbitmq"
	userobject "integration-project-ehb/controlroom/internal/userObject"

	elasticsearch "github.com/elastic/go-elasticsearch/v9"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {

	// =============================================================================
	// RabbitMQ connection
	conn, err := amqp.Dial(os.Getenv("RABBITMQ_URL"))
	if err != nil {
		log.Fatalf("RabbitMQ connection failed: %v", err)
	}
	defer conn.Close()

	ir := &cr_rabbitmq.InternalRabbitMQ{
		Conn:  conn,
		Chans: make(map[string]*amqp.Channel),
	}

	// =============================================================================

	// =============================================================================
	// RabbitMQ consumers
	msgs, err := cr_rabbitmq.SetupHeartbeatConsumer(ir)
	if err != nil {
		log.Fatalf("SetupHeartbeatConsumer failed: %v", err)
	}

	msgsUser, err := cr_rabbitmq.SetupUserConsumer(ir)
	if err != nil {
		log.Fatalf("SetupUserConsumer failed: %v", err)
	}

	defer func() {
		for _, ch := range ir.Chans {
			ch.Close()
		}
	}()
	// =============================================================================

	// =============================================================================
	// Elasticsearch
	esClient, err := elasticsearch.NewDefaultClient()
	if err != nil {
		log.Fatalf("Failed to create Elasticsearch client: %v", err)
	}
	// =============================================================================

	// =============================================================================
	res, err := esClient.Info()
	if err != nil {
		log.Fatalf("Error connecting to Elasticsearch: %v", err)
	}
	defer res.Body.Close()
	log.Println("Successfully connected to Elasticsearch!")
	// =============================================================================

	// =============================================================================
	// DLQ channel
	dlqCh, err := conn.Channel()
	if err != nil {
		log.Fatalf("Failed to open DLQ channel: %v", err)
	}
	defer dlqCh.Close()
	// =============================================================================

	// =============================================================================
	// Processors
	processor := heartbeat.CreateProcessor(esClient, dlqCh)
	processorUser := userobject.CreateProcessor(esClient, dlqCh)
	// =============================================================================

	// =============================================================================
	// Graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	// =============================================================================

	// =============================================================================
	go heartbeat.ConsumeHeartbeats(processor, msgs, ctx)
	go userobject.ConsumeUserObjects(processorUser, msgsUser, ctx)
	// =============================================================================

	// =============================================================================
	log.Println("Controlroom is running...")
	<-sigChan
	log.Println("Shutting down...")
	cancel()
}
