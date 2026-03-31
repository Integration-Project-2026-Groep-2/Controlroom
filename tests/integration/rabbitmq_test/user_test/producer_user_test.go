package user_test

import (
	"context"
	"encoding/xml"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	amqp "github.com/rabbitmq/amqp091-go"
	"integration-project-ehb/controlroom/pkg/xml/gen"
)

func TestProducerUser(t *testing.T) {
	validate := validator.New()

	// Load RabbitMQ connection from env or use default
	connStr := os.Getenv("RABBITMQ_URL")
	if connStr == "" {
		connStr = "amqp://guest:guest@127.0.0.1:5672"
	}
	log.Printf("got string")

	conn, err := amqp.Dial(connStr)
	if err != nil {
		log.Fatalf("❌ Failed to connect to RabbitMQ: %v", err)
	}

	log.Printf("connect rabbitmq")
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("Failed to open channel: %v", err)
	}
	defer ch.Close()

	err = ch.ExchangeDeclare("user.topic", "topic", true, false, false, false, nil)
	if err != nil {
		log.Fatalf("Failed to declare exchange: %v", err)
	}

	log.Printf("exchange declareed")

	// Declare queue for confirmed users
	q, err := ch.QueueDeclare(
		"crm.user.confirmed", // name
		true,                 // durable
		false,                // delete when unused
		false,                // exclusive
		false,                // no-wait
		nil,                  // arguments
	)
	if err != nil {
		log.Fatalf("Failed to declare queue: %v", err)
	}

	// Bind queue to exchange with correct routing key
	err = ch.QueueBind(
		q.Name,
		"crm.user.confirmed", // routing key matches the queue/event type
		"user.topic",
		false,
		nil,
	)
	if err != nil {
		log.Fatalf("Failed to bind queue to exchange: %v", err)
	}

	fmt.Println("Producer started! Sending data every 5 seconds... (Press CTRL+C to exit)")

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

		// Validate & send adminUser
		if err := validate.Struct(adminUser); err == nil {
			xmlData, _ := xml.Marshal(adminUser)
			fmt.Printf("Publishing: %s\n", string(xmlData)) // DEBUG
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
				log.Printf("Failed to publish admin user: %v", err)
			} else {
				fmt.Println("✓ Sent: admin user confirmed")
			}
		} else {
			log.Printf("Admin user validation failed: %v", err)
		}

		// Validate & send speakerUser
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
				log.Printf("Failed to publish speaker user: %v", err)
			} else {
				fmt.Println("✓ Sent: speaker user confirmed")
			}
		} else {
			log.Printf("Speaker user validation failed: %v", err)
		}

		time.Sleep(5 * time.Second)
	}
}
