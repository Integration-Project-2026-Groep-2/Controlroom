package heartbeat

import (
	"context"
	"encoding/xml"
	"fmt"
	"log"
	"time"

	elasticsearch "github.com/elastic/go-elasticsearch/v9"
	"github.com/go-playground/validator/v10"
	amqp "github.com/rabbitmq/amqp091-go"

	// generated structs
	"integration-project-ehb/controlroom/pkg/xml/gen"
)

type DLQPublisher interface {
	PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error
}

type Processor struct {
	es        *elasticsearch.Client
	validator *validator.Validate
	dlq       DLQPublisher
}

func NewProcessor(es *elasticsearch.Client, dlq DLQPublisher) *Processor {
	return &Processor{
		es:        es,
		validator: validator.New(),
		dlq:       dlq,
	}
}

func CreateProcessor(es *elasticsearch.Client, dlqCh *amqp.Channel) *Processor {
	_, err := dlqCh.QueueDeclare("heartbeat_dlq", true, false, false, false, nil)
	if err != nil {
		log.Printf("Failed to declare DLQ: %v", err)
	}

	return &Processor{
		es:        es,
		validator: validator.New(),
		dlq:       dlqCh,
	}
}

func ProcessHeartbeat(p *Processor, body []byte) error {
	var hb gen.Heartbeat

	if err := xml.Unmarshal(body, &hb); err != nil {
		return sendToDLQ(p.dlq, body, fmt.Sprintf("unmarshal error: %v", err))
	}

	if err := p.validator.Struct(hb); err != nil {
		return sendToDLQ(p.dlq, body, fmt.Sprintf("validation error: %v", err))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := indexHeartbeat(p.es, ctx, &hb); err != nil {
		return sendToDLQ(p.dlq, body, fmt.Sprintf("index error: %v", err))
	}

	log.Printf("Indexed heartbeat: %s", hb.ServiceId)
	return nil
}

func sendToDLQ(dlq DLQPublisher, body []byte, reason string) error {
	err := dlq.PublishWithContext(
		context.Background(),
		"",
		"heartbeat_dlq",
		false,
		false,
		amqp.Publishing{
			ContentType: "application/xml",
			Body:        body,
			Headers: amqp.Table{
				"error_reason": reason,
				"failed_at":    time.Now().Format(time.RFC3339),
			},
		},
	)

	if err != nil {
		log.Printf("Failed to send to DLQ: %v", err)
	}

	return err
}
