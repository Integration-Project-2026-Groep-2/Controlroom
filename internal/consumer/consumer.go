package consumer

import (
	"bytes"
	"context"
	// NOTE(nasr): json is needed because elasticsearch uses json over http
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log"

	"github.com/elastic/go-elasticsearch/v9"
	amqp "github.com/rabbitmq/amqp091-go"

)

func Start() {

	var consumer consumerType

	// Connect to Elasticsearch
	es, err := elasticsearch.NewDefaultClient()
	if err != nil {
		log.Fatalf("Error creating ES client: %s", err)
	}

	// Connect to RabbitMQ
	// TODO(nasr): handle this using env files 
	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		// TODO(nasr): send errors to kibana
		log.Fatalf("Failed to connect to RabbitMQ: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("Failed to open channel: %v", err)
	}
	defer ch.Close()


	// TODO(nasr): needs refactorign 
	ch.ExchangeDeclare("control_room_exchange", "direct", true, false, false, false, nil)
	qHeartbeat, _ := ch.QueueDeclare("heartbeat_queue", true, false, false, false, nil)
	qUser, _ := ch.QueueDeclare("user_queue", true, false, false, false, nil)
	ch.QueueBind(qHeartbeat.Name, "routing.heartbeat", "control_room_exchange", false, nil)
	ch.QueueBind(qUser.Name, "routing.user", "control_room_exchange", false, nil)
	msgsHeartbeat, _ := ch.Consume(qHeartbeat.Name, "", true, false, false, false, nil)
	msgsUser, _ := ch.Consume(qUser.Name, "", true, false, false, false, nil)


	validate := validator.New()

	go user.Validate()
	go heartbeat.Validate()

	select {}
}

// Helper to push to Elasticsearch
func sendToElastic(es *elasticsearch.Client, index string, data interface{}) {
	jsonValue, _ := json.Marshal(data)
	req := esapi.IndexRequest{
		Index:   index,
		Body:    bytes.NewReader(jsonValue),
		Refresh: "true",
	}
	res, _ := req.Do(context.Background(), es)
	if res != nil {
		res.Body.Close()
	}
}
