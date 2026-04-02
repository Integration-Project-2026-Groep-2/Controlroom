// controlroom entry point
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"integration-project-ehb/controlroom/internal/heartbeat"
	cr_rabbitmq "integration-project-ehb/controlroom/internal/rabbitmq"
	userobject "integration-project-ehb/controlroom/internal/userobject"
	internal_logger "integration-project-ehb/controlroom/pkg/logger"

	elasticsearch "github.com/elastic/go-elasticsearch/v9"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {

	// =============================================================================
	// Elasticsearch + logger eerst — alle fouten erna gaan naar Kibana
	esClient, err := elasticsearch.NewDefaultClient()
	if err != nil {
		log.Fatalf("Failed to create Elasticsearch client: %v", err)
	}

	res, err := esClient.Info()
	if err != nil {
		log.Fatalf("Error connecting to Elasticsearch: %v", err)
	}
	defer res.Body.Close()

	logger := internal_logger.NewWithElastic(os.Stdout, "controlroom", esClient, "logs")
	logger.Info("successfully connected to Elasticsearch")
	// =============================================================================

	// =============================================================================
	// RabbitMQ connection
	conn, err := amqp.Dial(os.Getenv("RABBITMQ_URL"))
	if err != nil {
		logger.Fatal("RabbitMQ connection failed", err,
			internal_logger.String("url", os.Getenv("RABBITMQ_URL")),
		)
	}
	defer conn.Close()
	logger.Info("successfully connected to RabbitMQ",
		internal_logger.String("host", os.Getenv("RABBITMQ_HOST")),
	)

	ir := &cr_rabbitmq.InternalRabbitMQ{
		Conn:  conn,
		Chans: make(map[string]*amqp.Channel),
	}
	// =============================================================================

	// =============================================================================
	// RabbitMQ consumers
	msgs, err := cr_rabbitmq.SetupHeartbeatConsumer(ir)
	if err != nil {
		logger.Fatal("SetupHeartbeatConsumer failed", err)
	}

	msgsUser, err := cr_rabbitmq.SetupUserConsumer(ir)
	if err != nil {
		logger.Fatal("SetupUserConsumer failed", err)
	}

	defer func() {
		for _, ch := range ir.Chans {
			ch.Close()
		}
	}()
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
	processor := heartbeat.CreateProcessor(esClient, dlqCh, logger)
	processorUser := userobject.CreateProcessor(esClient, dlqCh, logger)
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
	go heartbeat.ConsumeHeartbeats(processor, msgs, ctx, logger)
	go userobject.ConsumeUserObjects(processorUser, msgsUser, ctx, logger)
	// =============================================================================

	// =============================================================================
	logger.Info("controlroom is running")
	<-sigChan
	logger.Info("shutting down")
	cancel()
}
