package main

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/go-playground/validator/v10"
	amqp "github.com/rabbitmq/amqp091-go"
)

// --- DATA STRUCTURES ---
type Heartbeat struct {
	XMLName   xml.Name `xml:"Heartbeat" json:"-"`
	ServiceID string   `xml:"serviceId" json:"service_id" validate:"required"`
	Timestamp string   `xml:"timestamp" json:"@timestamp" validate:"required"`
}

type User struct {
	XMLName     xml.Name `xml:"User" json:"-"`
	ID          string   `xml:"id" json:"user_id" validate:"required"`
	Type        string   `xml:"type" json:"user_type" validate:"required,oneof=human service external"`
	Organisatie string   `xml:"organisatie" json:"organisatie" validate:"required"`
	Datum       string   `xml:"datum" json:"@timestamp" validate:"required"`
}

func main() {
	validate := validator.New()

	// 1. Connect to Elasticsearch
	es, err := elasticsearch.NewDefaultClient()
	if err != nil {
		log.Fatalf("❌ Error creating ES client: %s", err)
	}

	// 2. Connect to RabbitMQ
	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		log.Fatalf("❌ Failed to connect to RabbitMQ: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("Failed to open channel: %v", err)
	}
	defer ch.Close()

	// 3. Declare Exchange & Queues
	ch.ExchangeDeclare("control_room_exchange", "direct", true, false, false, false, nil)
	qHeartbeat, _ := ch.QueueDeclare("heartbeat_queue", true, false, false, false, nil)
	qUser, _ := ch.QueueDeclare("user_queue", true, false, false, false, nil)

	// 4. Bind Queues to Exchange using Routing Keys
	ch.QueueBind(qHeartbeat.Name, "routing.heartbeat", "control_room_exchange", false, nil)
	ch.QueueBind(qUser.Name, "routing.user", "control_room_exchange", false, nil)

	// 5. Consume from Queues
	msgsHeartbeat, _ := ch.Consume(qHeartbeat.Name, "", true, false, false, false, nil)
	msgsUser, _ := ch.Consume(qUser.Name, "", true, false, false, false, nil)

	fmt.Println("🎧 Control Room Consumer listening... (Press CTRL+C to exit)")

	// Go routine to process Heartbeats
	go func() {
		for d := range msgsHeartbeat {
			var hb Heartbeat
			xml.Unmarshal(d.Body, &hb)
			if err := validate.Struct(hb); err == nil {
				fmt.Printf("✅ Valid Heartbeat: %s\n", hb.ServiceID)
				sendToElastic(es, "heartbeats", hb)
			} else {
				fmt.Println("❌ Invalid Heartbeat received")
			}
		}
	}()

	// Go routine to process Users
	go func() {
		for d := range msgsUser {
			var u User
			xml.Unmarshal(d.Body, &u)
			if err := validate.Struct(u); err == nil {
				fmt.Printf("✅ Valid User: %s\n", u.ID)
				sendToElastic(es, "users", u)
			} else {
				fmt.Println("❌ Invalid User received")
			}
		}
	}()

	// Keep program running
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
