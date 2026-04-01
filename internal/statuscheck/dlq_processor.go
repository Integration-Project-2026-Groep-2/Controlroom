package statuscheck

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

func sendToDLQ(dlq *amqp.Channel, body []byte, reason string) error {
	err := dlq.PublishWithContext(context.Background(), "", "statuscheck_dlq", false, false,
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
