package producer_test

import (
	"context"
	"encoding/xml"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	amqp "github.com/rabbitmq/amqp091-go"

	"integration-project-ehb/controlroom/pkg/xml/gen"
)

func TestProducer(t *testing.T) {

	validate := validator.New()

	// 1. Connect to RabbitMQ
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

	// 2. Declare the Exchange
	err = ch.ExchangeDeclare("control_room_exchange", "direct", true, false, false, false, nil)
	if err != nil {
		log.Fatalf("Failed to declare exchange: %v", err)
	}

	fmt.Println("🚀 Producer started! Sending data every 5 seconds... (Press CTRL+C to exit)")

	// 3. The Infinite Sending Loop
	for {
		now := time.Now().UTC()

		// Create dummy data
		hbCRM := gen.Heartbeat{ServiceId: "Service-CRM", Timestamp: now}
		hbFacturatie := gen.Heartbeat{ServiceId: "Service-Facturatie", Timestamp: now}
		hbMailing := gen.Heartbeat{ServiceId: "Service-Mailing", Timestamp: now}
		hbPlanning := gen.Heartbeat{ServiceId: "Service-Planning", Timestamp: now}
		hbInfra := gen.Heartbeat{ServiceId: "Service-Ifra", Timestamp: now}
		hbFrontend := gen.Heartbeat{ServiceId: "Service-Frontend", Timestamp: now}

		// Validate & Send CRM Heartbeat
		if err := validate.Struct(hbCRM); err == nil {
			xmlData, _ := xml.Marshal(hbCRM)
			ch.PublishWithContext(context.Background(), "control_room_exchange", "routing.heartbeat", false, false, amqp.Publishing{ContentType: "text/xml", Body: xmlData})
			fmt.Println("📤 Sent: CRM Heartbeat")
		}

		// Validate & Send Facturatie Heartbeat
		if err := validate.Struct(hbFacturatie); err == nil {
			xmlData, _ := xml.Marshal(hbFacturatie)
			ch.PublishWithContext(context.Background(), "control_room_exchange", "routing.heartbeat", false, false, amqp.Publishing{ContentType: "text/xml", Body: xmlData})
			fmt.Println("📤 Sent: Facturatie Heartbeat")
		}

		// Validate & Send Mailing Heartbeat
		if err := validate.Struct(hbMailing); err == nil {
			xmlData, _ := xml.Marshal(hbMailing)
			ch.PublishWithContext(context.Background(), "control_room_exchange", "routing.heartbeat", false, false, amqp.Publishing{ContentType: "text/xml", Body: xmlData})
			fmt.Println("📤 Sent: Mailing Heartbeat")
		}

		// Validate & Send Planning Heartbeat
		if err := validate.Struct(hbPlanning); err == nil {
			xmlData, _ := xml.Marshal(hbPlanning)
			ch.PublishWithContext(context.Background(), "control_room_exchange", "routing.heartbeat", false, false, amqp.Publishing{ContentType: "text/xml", Body: xmlData})
			fmt.Println("📤 Sent: Planning Heartbeat")
		}

		// Validate & Send Infra Heartbeat
		if err := validate.Struct(hbInfra); err == nil {
			xmlData, _ := xml.Marshal(hbInfra)
			ch.PublishWithContext(context.Background(), "control_room_exchange", "routing.heartbeat", false, false, amqp.Publishing{ContentType: "text/xml", Body: xmlData})
			fmt.Println("📤 Sent: Infra Heartbeat")
		}

		// Validate & Send Frontend Heartbeat
		if err := validate.Struct(hbFrontend); err == nil {
			xmlData, _ := xml.Marshal(hbFrontend)
			ch.PublishWithContext(context.Background(), "control_room_exchange", "routing.heartbeat", false, false, amqp.Publishing{ContentType: "text/xml", Body: xmlData})
			fmt.Println("📤 Sent: Frontend Heartbeat")
		}

		fmt.Println("--- Waiting 5 seconds ---")
		time.Sleep(5 * time.Second)
	}
}
