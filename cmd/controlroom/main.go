package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	company "integration-project-ehb/controlroom/internal/company"
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

	Consumer := &cr_rabbitmq.ConsumerInfo{
		Name:      "heartbeat",
		Consumer:  "",
		AutoAck:   false,
		Exclusive: false,
		NoLocal:   false,
		NoWait:    false,
		Args:      nil,
	}

	Exchange := &cr_rabbitmq.Exchange{
		Name:       "heartbeat.direct",
		Kind:       "direct",
		Durable:    true,
		AutoDelete: false,
		Internal:   false,
		NoWait:     false,
		Args:       nil,
	}

	Queue := &cr_rabbitmq.Queue{
		Name:       "heartbeat_queue",
		Durable:    true,
		AutoDelete: false,
		Exclusive:  false,
		NoWait:     false,
		Args:       nil,
	}

	Bind := &cr_rabbitmq.BindInfo{
		Key:    "routing.heartbeat",
		NoWait: false,
		Args:   nil,
	}

	// TODO(nasr): verify prefetch count with team (currently 6 for 6 microservices)
	Qos := &cr_rabbitmq.Qos{
		PrefetchCount: 6,
		PrefetchSize:  0,
		Global:        true,
	}

	msgs, err := cr_rabbitmq.SetupConsumer(ir, Consumer, Exchange, Queue, Bind, Qos)
	if err != nil {
		log.Fatalf("SetupHeartbeatConsumer failed: %v", err)
	}

	Consumer.Name = "user"
	Exchange.Name = "contact.topic"
	Exchange.Kind = "topic"
	Queue.Name = "crm.user.confirmed"
	// TODO(Steven): Add actual routing key when exists
	Bind.Key = "temp.routing.users"
	// TODO(nasr): verify prefetch count with team
	Qos.PrefetchCount = 10
	Qos.Global = false

	msgsUser, err := cr_rabbitmq.SetupConsumer(ir, Consumer, Exchange, Queue, Bind, Qos)
	if err != nil {
		log.Fatalf("SetupUserConsumer failed: %v", err)
	}

	Consumer.Name = "company"
	Exchange.Name = "contact.topic"
	Exchange.Kind = "topic"
	Queue.Name = "crm.company.confirmed"
	// TODO(Steven): Add actual routing key when exists
	Bind.Key = "temp.routing.users"
	// TODO(nasr): verify prefetch count with team
	Qos.PrefetchCount = 10
	Qos.Global = false

	msgsCompany, err := cr_rabbitmq.SetupConsumer(ir, Consumer, Exchange, Queue, Bind, Qos)
	if err != nil {
		log.Fatalf("SetupCompanyConsumer failed: %v", err)
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
	processorCompany := company.CreateProcessor(esClient, dlqCh)
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
	go company.ConsumeCompanies(processorCompany, msgsCompany, ctx)
	// =============================================================================

	// =============================================================================
	log.Println("Controlroom is running...")
	<-sigChan
	log.Println("Shutting down...")
	cancel()
}
