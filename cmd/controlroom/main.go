// controlroom entry point
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	// log blijft voor de Fatalf calls vóór de logger beschikbaar is
	"integration-project-ehb/controlroom/internal/heartbeat"
	cr_rabbitmq "integration-project-ehb/controlroom/internal/rabbitmq"
	userobject "integration-project-ehb/controlroom/internal/userObject"
	internal_logger "integration-project-ehb/controlroom/pkg/logger"
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

	logger := internal_logger.NewWithElastic(os.Stdout, "controlroom", esClient, "logs")
	logger.Info("successfully connected to Elasticsearch")
	// =============================================================================

	// =============================================================================
	// DLQ channel
	dlqCh, err := conn.Channel()
	if err != nil {
		logger.Fatal("failed to open DLQ channel", err)
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

	// NOTE(nasr): docker and kuberenets send a SIGTERM the sigChan keeps opens a channel
	// that listens for that exact signal and for the time it hasn't received it
	// the program keeps running
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	// =============================================================================

	// =============================================================================
	go heartbeat.ConsumeHeartbeats(processor, msgs, ctx)
	go userobject.ConsumeUserObjects(processorUser, msgsUser, ctx)
	// =============================================================================

	// =============================================================================
	logger.Info("controlroom is running")
	<-sigChan
	logger.Info("shutting down")
	cancel()
}
