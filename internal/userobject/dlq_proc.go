package userobject

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
	"integration-project-ehb/controlroom/internal/cr_rabbitmq"
	"integration-project-ehb/controlroom/pkg/xml/gen"
)

func ProcessUserObject(p *cr_rabbitmq.Processor, body []byte) error {
	var uo gen.UserConfirmed

	if err := xml.Unmarshal(body, &uo); err != nil {
		return sendToDLQ(p.Dlq, body, fmt.Sprintf("unmarshal error: %v", err))
	}

	if err := p.Validator.Struct(uo); err != nil {
		return sendToDLQ(p.Dlq, body, fmt.Sprintf("validation error: %v", err))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := indexUser(p.Es, ctx, &uo); err != nil {
		return sendToUserDLQ(p.Dlq, body, fmt.Sprintf("index error: %v", err))
	}

	log.Printf("Indexed User: %s", uo.Id)
	return nil
}

func sendToUserDLQ(dlq *amqp.Channel, body []byte, reason string) error {
	err := dlq.PublishWithContext(context.Background(), "", "user_dlq", false, false,
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
