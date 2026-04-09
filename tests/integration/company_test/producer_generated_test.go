package company_test

import (
	"encoding/xml"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	"integration-project-ehb/controlroom/pkg/gen"
)

type CompanyConfirmed struct {
	XMLName     xml.Name                `xml:"CompanyConfirmed"`
	ID          gen.UUIDType            `xml:"id"`
	VatNumber   string                  `xml:"vatNumber"`
	Name        string                  `xml:"name"`
	Email       gen.EmailType           `xml:"email"`
	IsActive    bool                    `xml:"isActive"`
	ConfirmedAt gen.ISO8601DateTimeType `xml:"confirmedAt"`
}

func TestPublishCompanyConfirmed(t *testing.T) {
	// Connect to RabbitMQ
	conn, err := amqp.Dial(os.Getenv("RABBITMQ_URL"))
	if err != nil {
		t.Fatalf("Failed to connect to RabbitMQ: %v", err)
	}
	defer conn.Close()

	// Open a channel
	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("Failed to open channel: %v", err)
	}
	defer ch.Close()

	// Declare the exchange
	err = ch.ExchangeDeclare(
		"contact.topic", // name
		"topic",         // kind
		true,            // durable
		false,           // auto-delete
		false,           // internal
		false,           // no-wait
		nil,             // args
	)
	if err != nil {
		t.Fatalf("Failed to declare exchange: %v", err)
	}

	// Produce 10 company.confirmed messages
	for i := 1; i <= 10; i++ {
		company := CompanyConfirmed{
			ID:          gen.UUIDType(uuid.New().String()),
			VatNumber:   fmt.Sprintf("BE%010d", i),
			Name:        fmt.Sprintf("Company %d", i),
			Email:       gen.EmailType(fmt.Sprintf("company%d@example.com", i)),
			IsActive:    true,
			ConfirmedAt: gen.ISO8601DateTimeType(time.Now().UTC().Format(time.RFC3339)),
		}

		// Marshal to XML
		xmlData, err := xml.MarshalIndent(company, "", "  ")
		if err != nil {
			t.Fatalf("Failed to marshal XML: %v", err)
		}

		// Publish message
		err = ch.Publish(
			"contact.topic",           // exchange
			"crm.company.confirmed",   // routing key
			false,                     // mandatory
			false,                     // immediate
			amqp.Publishing{
				ContentType: "application/xml",
				Body:        xmlData,
			},
		)
		if err != nil {
			t.Fatalf("Failed to publish message: %v", err)
		}

		t.Logf("Published company %d: %s (VAT: %s)", i, company.Name, company.VatNumber)
	}

	t.Log("Successfully published 10 company.confirmed messages!")
}
