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

	// 2. Declare the Exchange
	err = ch.ExchangeDeclare("control_room_exchange", "direct", true, false, false, false, nil)
	if err != nil {
		log.Fatalf("Failed to declare exchange: %v", err)
	}

	fmt.Println("Producer started! Sending data every 5 seconds... (Press CTRL+C to exit)")

	for range 10 {
		now := time.Now().UTC()

		hbCRM := gen.Heartbeat{ServiceId: "SuperSecretService", Timestamp: now}
		hbFacturatie := gen.Heartbeat{ServiceId: "SuperNonSecretService", Timestamp: now}

		if err := validate.Struct(hbCRM); err == nil {
			xmlData, _ := xml.Marshal(hbCRM)
			ch.PublishWithContext(context.Background(), "control_room_exchange", "routing.heartbeat", false, false, amqp.Publishing{ContentType: "text/xml", Body: xmlData})
			fmt.Println("Sent: CRM Heartbeat")
		}

		if err := validate.Struct(hbFacturatie); err == nil {
			xmlData, _ := xml.Marshal(hbFacturatie)
			ch.PublishWithContext(context.Background(), "control_room_exchange", "routing.heartbeat", false, false, amqp.Publishing{ContentType: "text/xml", Body: xmlData})
			fmt.Println("Sent: Facturatie Heartbeat")
		}

		time.Sleep(5 * time.Second)
	}
}
