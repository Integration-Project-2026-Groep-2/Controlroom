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

	gotdotenv "github.com/joho/godotenv"

	"integration-project-ehb/controlroom/pkg/xml/gen"
)

func TestProducerCompany(t *testing.T) {
	//Use env file to load variables in tests
	err := gotdotenv.Load("../../../../.env")
	if err != nil {
		log.Fatal(err)
	}

	// For testing, we will assume the host will be local host, and the standard rabbitMQ port :)
	host := "127.0.0.1:5672"
	user := os.Getenv("RABBITMQ_USER")
	password := os.Getenv("RABBITMQ_PASSWORD")

	connectionString := fmt.Sprintf("amqp://%s:%s@%s", user, password, host)

	validate := validator.New()

	//TODO(steven): find way to do this with env variable
	//connectionString := fmt.Sprintf("amqp://root:admin@127.0.0.1:5672")
	conn, err := amqp.Dial(connectionString)
	if err != nil {
		log.Fatalf("❌ Failed to connect to RabbitMQ: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("Failed to open channel: %v", err)
	}
	defer ch.Close()

	// NOTE(Steven): move Queue and exchange names to seperate scheme files
	err = ch.ExchangeDeclare("contact.topic", "topic", true, false, false, false, nil)
	if err != nil {
		log.Fatalf("Failed to declare exchange: %v", err)
	}

	// NOTE(Steven): move Queue and exchange names to seperate scheme files
	q, err := ch.QueueDeclare(
		"crm.company.confirmed", // name
		true,                    // durable (survives server restarts)
		false,                   // delete when unused
		false,                   // exclusive (only used by this connection)
		false,                   // no-wait
		nil,                     // arguments
	)
	if err != nil {
		log.Fatalf("Failed to declare a queue: %v", err)
	}

	// TODO(steven): change routing key to actual key
	// Routing key: "temp.routing.consumers"
	err = ch.QueueBind(
		q.Name,                   // queue name
		"temp.routing.companies", // routing key
		"contact.topic",          // exchange
		false,
		nil,
	)

	if err != nil {
		log.Fatalf("Failed to bind queue to exchange: %v", err)
	}

	fmt.Println("Producer started! Sending data every 5 seconds... (Press CTRL+C to exit)")

	for range 10 {

		Company1 := gen.CompanyConfirmed{
			Id:          gen.UUIDType("550e8400-e29b-41d4-a716-446655440000"),
			VatNumber:   gen.BelgianVatNumberType("BE3709455935"),
			Name:        "Doe Industries",
			Email:       "admin@company.com",
			IsActive:    true,
			ConfirmedAt: gen.ISO8601DateTimeType(time.Now().Format(time.RFC3339)),
		}
		Company2 := gen.CompanyConfirmed{
			Id:          gen.UUIDType("a3b8c9d0-1234-5678-90ab-cdef12345678"),
			VatNumber:   gen.BelgianVatNumberType("BE3709455735"),
			Name:        "Jane Inc.",
			Email:       "John@Jane.com",
			IsActive:    false,
			ConfirmedAt: gen.ISO8601DateTimeType(time.Now().Format(time.RFC3339)),
		}

		// Validate & Send Company1
		// TODO(steven): replace temp routing key with actual key
		if err := validate.Struct(Company1); err == nil {
			xmlData, _ := xml.Marshal(Company1)
			ch.PublishWithContext(context.Background(), "contact.topic", "temp.routing.companies", false, false, amqp.Publishing{ContentType: "text/xml", Body: xmlData})
			fmt.Println("Sent: Company1 recieved")
		}

		// Validate & Send Company2
		// TODO(steven): replace temp routing key with actual key

		if err := validate.Struct(Company2); err == nil {
			xmlData, _ := xml.Marshal(Company2)
			ch.PublishWithContext(context.Background(), "contact.topic", "temp.routing.companies", false, false, amqp.Publishing{ContentType: "text/xml", Body: xmlData})
			fmt.Println("Sent: Company2 recieved")
		}

		time.Sleep(1 * time.Second)
	}

}
