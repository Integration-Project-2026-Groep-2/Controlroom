package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"integration-project-ehb/controlroom/internal/heartbeat"
	"integration-project-ehb/controlroom/internal/rabbitmq"

	elasticsearch "github.com/elastic/go-elasticsearch/v9"
)

func main() {
	log.Println("Starting Controlroom...")

	// RabbitMQ setup
	conn, ch, msgs, err := cr_rabbitmq.SetupHeartbeatConsumer()
	if err != nil {
		log.Fatalf("RabbitMQ setup failed: %v", err)
	}
	defer conn.Close()
	defer ch.Close()

	// Elasticsearch
	esClient, err := elasticsearch.NewDefaultClient()
	if err != nil {
		log.Fatalf("Failed to create Elasticsearch client: %v", err)
	}

	// DLQ channel
	dlqCh, err := conn.Channel()
	if err != nil {
		log.Fatalf("Failed to open DLQ channel: %v", err)
	}
	defer dlqCh.Close()

	processor := heartbeat.CreateProcessor(esClient, dlqCh)

	// Graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go heartbeat.ConsumeHeartbeats(processor, msgs, ctx)

	log.Println("Controlroom is running...")

	<-sigChan
	log.Println("Shutting down...")
	cancel()
}
