package main

import (
	"log"

	"integration-project-ehb/controlroom/internal/heartbeat"
	"integration-project-ehb/controlroom/internal/user"

	elasticsearch "github.com/elastic/go-elasticsearch/v9"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {

	// Connect to RabbitMQ
	// TODO(nasr): handle credentials using github secrets
	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("Failed to open channel: %v", err)
	}
	defer ch.Close()

	// Declare exchange
	err = ch.ExchangeDeclare("control_room_exchange", "direct", true, false, false, false, nil)
	if err != nil {
		log.Fatalf("Failed to declare exchange: %v", err)
	}

	// Declare queues
	qHeartbeat, err := ch.QueueDeclare("heartbeat_queue", false, true, false, false, nil)
	if err != nil {
		log.Fatalf("Failed to declare heartbeat queue: %v", err)
	}

	qUser, err := ch.QueueDeclare("user_queue", true, false, false, false, nil)
	if err != nil {
		log.Fatalf("Failed to declare user queue: %v", err)
	}

	// Bind queues to exchange
	if err = ch.QueueBind(qHeartbeat.Name, "routing.heartbeat", "control_room_exchange", false, nil); err != nil {
		log.Fatalf("Failed to bind heartbeat queue: %v", err)
	}
	if err = ch.QueueBind(qUser.Name, "routing.user", "control_room_exchange", false, nil); err != nil {
		log.Fatalf("Failed to bind user queue: %v", err)
	}

	// Start consumers
	msgsHeartbeat, err := ch.Consume(qHeartbeat.Name, "", true, false, false, false, nil)
	if err != nil {
		log.Fatalf("Failed to consume heartbeat queue: %v", err)
	}

	msgsUser, err := ch.Consume(qUser.Name, "", true, false, false, false, nil)
	if err != nil {
		log.Fatalf("Failed to consume user queue: %v", err)
	}

	// Connect to Elasticsearch
	// TODO(nasr): send errors to kibana
	esClient, err := elasticsearch.NewDefaultClient()
	if err != nil {
		log.Fatalf("Failed to create Elasticsearch client: %v", err)
	}

	go heartbeat.Validate(msgsHeartbeat, esClient)
	go user.Validate(msgsUser, esClient)

	select {}
}
