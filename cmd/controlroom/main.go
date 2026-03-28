package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"integration-project-ehb/controlroom/internal/heartbeat"
	cr_rabbitmq "integration-project-ehb/controlroom/internal/rabbitmq"
	userobject "integration-project-ehb/controlroom/internal/userObject"

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

	// Setup consumer of user
	connUser, chUser, msgsUser, errUser := cr_rabbitmq.SetupUserConsumer()
	if err != nil {
		log.Fatalf("setup of User Consumer failed: %v", errUser)
	}
	defer connUser.Close()
	defer chUser.Close()

	fmt.Printf("2 we are here")
	// Setup consumer of user
	connUser, chUser, msgsUser, errUser := cr_rabbitmq.SetupUserConsumer()
	if err != nil {
		log.Fatalf("setup of User Consumer failed: %v", errUser)
	}
	defer connUser.Close()
	defer chUser.Close()

	// Elasticsearch
	esClient, err := elasticsearch.NewDefaultClient()
	if err != nil {
		log.Fatalf("Failed to create Elasticsearch client: %v", err)
	}
	res, err := esClient.Info()
	if err != nil {
		log.Fatalf("Error connecting to Elasticsearch: %s", err)
	}
	defer res.Body.Close()
	log.Println("Successfully connected to Elasticsearch!")

	// DLQ channel
	dlqCh, err := conn.Channel()
	if err != nil {
		log.Fatalf("Failed to open DLQ channel: %v", err)
	}
	defer dlqCh.Close()

	processor := heartbeat.CreateProcessor(esClient, dlqCh)
	processorUser := userobject.CreateProcessor(esClient, dlqCh)

	// Graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go heartbeat.ConsumeHeartbeats(processor, msgs, ctx)
	go userobject.ConsumeUserObjects(processorUser, msgsUser, ctx)

	log.Println("Controlroom is running...")

	<-sigChan
	log.Println("Shutting down...")
	cancel()
}
