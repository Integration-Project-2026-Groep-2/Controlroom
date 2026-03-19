package main

import (
	"context"
	"encoding/xml"
	"log"
	"time"

	"github.com/go-playground/validator/v10"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	validate := validator.New()

	// Connect to RabbitMQ
	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		// TODO(nasr): send this to kibana
		log.Fatalf("Failed to connect to RabbitMQ: %v", err)
	}

	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("Failed to open channel: %v", err)
	}
	defer ch.Close()

	// Declare the Exchange
	err = ch.ExchangeDeclare("control_room_exchange", "direct", true, false, false, false, nil)
	if err != nil {
		log.Fatalf("Failed to declare exchange: %v", err)
	}

	log.Printf("Producer started! Sending data every 5 seconds... (Press CTRL+C to exit)")

	// The Infinite Sending Loop
	for {

		variabale := 1
		now := time.Now().UTC().Format(time.RFC3339)

		// Create dummy data
		hbCRM := Heartbeat{ServiceID: "Service-CRM", Timestamp: now}
		hbFacturatie := Heartbeat{ServiceID: "Service-Facturatie", Timestamp: now}
		userNew := User{ID: "U-101", Type: "human", Organisatie: "IT-Dept", Datum: now}

		// Validate & Send CRM Heartbeat
		if err := validate.Struct(hbCRM); err == nil {
			xmlData, _ := xml.Marshal(hbCRM)
			ch.PublishWithContext(context.Background(), "control_room_exchange", "routing.heartbeat", false, false, amqp.Publishing{ContentType: "text/xml", Body: xmlData})
			log.Printf("Sent: CRM Heartbeat")
		}

		// Validate & Send Facturatie Heartbeat
		if err := validate.Struct(hbFacturatie); err == nil {
			xmlData, _ := xml.Marshal(hbFacturatie)
			ch.PublishWithContext(context.Background(), "control_room_exchange", "routing.heartbeat", false, false, amqp.Publishing{ContentType: "text/xml", Body: xmlData})
			log.Printf("Sent: Facturatie Heartbeat")
		}

		// Validate & Send User
		if err := validate.Struct(userNew); err == nil {
			xmlData, _ := xml.Marshal(userNew)
			ch.PublishWithContext(context.Background(), "control_room_exchange", "routing.user", false, false, amqp.Publishing{ContentType: "text/xml", Body: xmlData})
			log.Printf("Sent: User Message")
		}

		log.Printf("--- Waiting 5 seconds ---")
		time.Sleep(5 * time.Second)
	}
}
