package user_test

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

func TestProducerUser(t *testing.T) {

	validate := validator.New()

	// 1. Connect to RabbitMQ
	conn, err := amqp.Dial("amqp://guest:guest@localhost:5671/")
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
	err = ch.ExchangeDeclare("user.topic", "direct", true, false, false, false, nil)
	if err != nil {
		log.Fatalf("Failed to declare exchange: %v", err)
	}

	fmt.Println("🚀 Producer started! Sending data every 5 seconds... (Press CTRL+C to exit)")

	// 3. The Infinite Sending Loop
	for {

		// Create dummy data
		adminUser := gen.UserConfirmed{
			Id:          gen.UUIDType("550e8400-e29b-41d4-a716-446655440000"),
			Email:       gen.EmailType("admin@event-platform.com"),
			FirstName:   "Jane",
			LastName:    "Doe",
			Phone:       "+1-555-0123",
			Role:        gen.UserRoleTypeADMIN,
			CompanyId:   gen.UUIDType("00000000-0000-0000-0000-000000000000"), // No specific company
			BadgeCode:   "ADMIN-001",
			IsActive:    true,
			GdprConsent: true,
			ConfirmedAt: gen.ISO8601DateTimeType(time.Now().Format(time.RFC3339)),
		}
		speakerUser := gen.UserConfirmed{
			Id:          gen.UUIDType("a3b8c9d0-1234-5678-90ab-cdef12345678"),
			Email:       gen.EmailType("tech.speaker@partner.com"),
			FirstName:   "Alex",
			LastName:    "Smith",
			Phone:       "+44-20-7946-0958",
			Role:        gen.UserRoleTypeSPEAKER,
			CompanyId:   gen.UUIDType("f47ac10b-58cc-4372-a567-0e02b2c3d479"),
			BadgeCode:   "SPKR-2026-X",
			IsActive:    true,
			GdprConsent: true,
			ConfirmedAt: gen.ISO8601DateTimeType("2026-03-28T13:45:00Z"),
		}

		// Validate & Send adminUser
		// TODO(steven): replace temp routing key with actual key
		if err := validate.Struct(adminUser); err == nil {
			xmlData, _ := xml.Marshal(adminUser)
			ch.PublishWithContext(context.Background(), "user.topic", "temp.routing.consumers", false, false, amqp.Publishing{ContentType: "text/xml", Body: xmlData})
			fmt.Println("📤 Sent: CRM Heartbeat")
		}

		// Validate & Send speakerUser
		// TODO(steven): replace temp routing key with actual key

		if err := validate.Struct(speakerUser); err == nil {
			xmlData, _ := xml.Marshal(speakerUser)
			ch.PublishWithContext(context.Background(), "user.topic", "temp.routing.consumers", false, false, amqp.Publishing{ContentType: "text/xml", Body: xmlData})
			fmt.Println("📤 Sent: Facturatie Heartbeat")
		}

		fmt.Println("--- Waiting 5 seconds ---")
		time.Sleep(5 * time.Second)
	}

}
