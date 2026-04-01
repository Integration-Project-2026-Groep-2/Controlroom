package cr_rabbitmq

import (
	"context"
	"encoding/xml"
	"fmt"
	"log"
	"time"

	elasticsearch "github.com/elastic/go-elasticsearch/v9"
	"github.com/go-playground/validator/v10"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Processor struct {
	Es        *elasticsearch.Client
	Validator *validator.Validate
	DLQ       *amqp.Channel
}

func NewProcessor(es *elasticsearch.Client, dlq *amqp.Channel) *Processor {
	return &Processor{
		Es:        es,
		Validator: validator.New(),
		DLQ:       dlq,
	}
}

func ProcessMessage(p *Processor, body []byte) error {
	var uo UserConfirmed // TODO(nasr): import from gen package
	if err := xml.Unmarshal(body, &uo); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}

	if err := p.Validator.Struct(uo); err != nil {
		return fmt.Errorf("validation: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := indexUser(p.Es, ctx, &uo); err != nil {
		return fmt.Errorf("index: %w", err)
	}

	return nil
}

// sendToDLQ publishes a message to the dead letter queue with error context.
func sendToDLQ(ch *amqp.Channel, body []byte, reason string) error {
	return ch.PublishWithContext(
		context.Background(),
		"",                // exchange
		"user_dlq",        // routing key
		false,             // mandatory
		false,             // immediate
		amqp.Publishing{
			ContentType: "application/octet-stream",
			Body:        body,
			Headers: amqp.Table{
				"error_reason": reason,
				"timestamp":    time.Now().Unix(),
			},
		},
	)
}

// SetupDLQ declares the dead letter queue. Call once at startup.
func SetupDLQ(dlq *amqp.Channel) error {
	_, err := dlq.QueueDeclare("user_dlq", true, false, false, false, nil)
	return err
}

// ConsumeMessages reads from a delivery channel and processes each message.
// On success, acks the message. On error, nacks and sends to DLQ.
func ConsumeMessages(p *Processor, msgs <-chan amqp.Delivery, ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Println("Message consumer shutting down...")
			return
		case msg, ok := <-msgs:
			if !ok {
				return
			}

			err := ProcessMessage(p, msg.Body)
			if err != nil {
				log.Printf("Process failed: %v", err)
				// Send to DLQ before nack
				if dlqErr := sendToDLQ(p.DLQ, msg.Body, err.Error()); dlqErr != nil {
					log.Printf("Failed to send to DLQ: %v", dlqErr)
				}
				msg.Nack(false, false)
			} else {
				msg.Ack(false)
			}
		}
	}
}

