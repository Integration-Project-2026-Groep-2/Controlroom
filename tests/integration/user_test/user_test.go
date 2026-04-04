package user_test

import (
	"context"
	"encoding/xml"
	"os"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	amqp "github.com/rabbitmq/amqp091-go"
	"integration-project-ehb/controlroom/pkg/gen"
)

func TestProducerUser(t *testing.T) {

	validate := validator.New()

	connStr := os.Getenv("RABBITMQ_URL")
	if connStr == "" {
		connStr = "amqp://guest:guest@127.0.0.1:5672"
	}

	conn, err := amqp.Dial(connStr)
	if err != nil {
		t.Fatalf("Failed to connect to RabbitMQ: %v", err)
	}

	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("Failed to open channel: %v", err)
	}
	defer ch.Close()

	err = ch.ExchangeDeclare("user.topic", "topic", false, false, false, false, nil)
	if err != nil {
		t.Fatalf("Failed to declare exchange: %v", err)
	}

	t.Log("exchange declared")

	// TODO(nasr): replace with steven struct
	q, err := ch.QueueDeclare("crm.user.confirmed", true, false, false, false, nil)

	if err != nil {
		t.Fatalf("Failed to declare queue: %v", err)
	}

	err = ch.QueueBind(q.Name, "crm.user.confirmed", "user.topic", false, nil)
	if err != nil {
		t.Fatalf("Failed to bind queue to exchange: %v", err)
	}

	t.Log("Producer started! Sending data every 5 seconds...")

	for range 10 {
		// Admin user
		adminUser := gen.UserConfirmed{
			Id:          gen.UUIDType("550e8400-e29b-41d4-a716-446655440000"),
			Email:       gen.EmailType("admin@event-platform.com"),
			FirstName:   "Jane",
			LastName:    "Doe",
			IsActive:    true,
			GdprConsent: true,
			Role:        gen.UserRoleTypeADMIN,
			ConfirmedAt: gen.ISO8601DateTimeType(time.Now().Format(time.RFC3339)),
		}

		// Speaker user
		speakerUser := gen.UserConfirmed{
			Id:          gen.UUIDType("a3b8c9d0-1234-5678-90ab-cdef12345678"),
			Email:       gen.EmailType("tech.speaker@partner.com"),
			FirstName:   "Alex",
			LastName:    "Smith",
			IsActive:    true,
			GdprConsent: true,
			Role:        gen.UserRoleTypeSPEAKER,
			ConfirmedAt: gen.ISO8601DateTimeType("2026-03-28T13:45:00Z"),
		}

		if err := validate.Struct(adminUser); err == nil {
			xmlData, _ := xml.Marshal(adminUser)
			t.Logf("Publishing: %s", string(xmlData))
			err := ch.PublishWithContext(
				context.Background(),
				"user.topic",
				"crm.user.confirmed",
				false,
				false,
				amqp.Publishing{
					ContentType: "text/xml",
					Body:        xmlData,
				},
			)
			if err != nil {
				t.Logf("Failed to publish admin user: %v", err)
			} else {
				t.Log("Sent: admin user confirmed")
			}
		} else {
			t.Logf("Admin user validation failed: %v", err)
		}

		if err := validate.Struct(speakerUser); err == nil {
			xmlData, _ := xml.Marshal(speakerUser)
			err := ch.PublishWithContext(
				context.Background(),
				"user.topic",
				"crm.user.confirmed",
				false,
				false,
				amqp.Publishing{
					ContentType: "text/xml",
					Body:        xmlData,
				},
			)
			if err != nil {
				t.Logf("Failed to publish speaker user: %v", err)
			} else {
				t.Log("Sent: speaker user confirmed")
			}
		} else {
			t.Logf("Speaker user validation failed: %v", err)
		}

		time.Sleep(5 * time.Second)
	}
}
