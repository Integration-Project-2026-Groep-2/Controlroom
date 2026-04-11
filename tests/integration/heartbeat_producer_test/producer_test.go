package producer_test

import (
	"context"
	"encoding/xml"
	"fmt"
	"log"
	"testing"
	"time"

	"integration-project-ehb/controlroom/pkg/gen"

	"github.com/go-playground/validator/v10"
	amqp "github.com/rabbitmq/amqp091-go"
)

func TestEndlessSimulator(t *testing.T) {
	validate := validator.New()

	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		log.Fatalf("RabbitMQ connection failed: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("Channel failed: %v", err)
	}
	defer ch.Close()

	ch.ExchangeDeclare("heartbeat.direct", "direct", true, false, false, false, nil)

	fmt.Println("🚀 Endless Simulator started! Sending 1 heartbeat/sec... (Press CTRL+C to stop)")

	for {
		now := time.Now().UTC()

		// Using DUMMY names so we don't mess with real data
		hbCRM := gen.Heartbeat{ServiceId: "DUMMY_CRM_SERVICE", Timestamp: now}
		hbFrontend := gen.Heartbeat{ServiceId: "DUMMY_FRONTEND_SERVICE", Timestamp: now}

		if err := validate.Struct(hbCRM); err == nil {
			xmlData, _ := xml.Marshal(hbCRM)
			ch.PublishWithContext(context.Background(), "heartbeat.direct", "routing.heartbeat", false, false, amqp.Publishing{ContentType: "text/xml", Body: xmlData})
		}

		if err := validate.Struct(hbFrontend); err == nil {
			xmlData, _ := xml.Marshal(hbFrontend)
			ch.PublishWithContext(context.Background(), "heartbeat.direct", "routing.heartbeat", false, false, amqp.Publishing{ContentType: "text/xml", Body: xmlData})
		}

		// Send every 1 second
		time.Sleep(1 * time.Second)
	}
}
