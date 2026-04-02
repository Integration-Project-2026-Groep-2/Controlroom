package userobject

import (
	"context"
	"encoding/xml"
	"fmt"
	"time"

	elasticsearch "github.com/elastic/go-elasticsearch/v9"
	"github.com/go-playground/validator/v10"
	amqp "github.com/rabbitmq/amqp091-go"

	// generated structs
	internal_logger "integration-project-ehb/controlroom/pkg/logger"
	"integration-project-ehb/controlroom/pkg/xml/gen"
)

type Processor struct {
	Es        *elasticsearch.Client
	Validator *validator.Validate
	Dlq       *amqp.Channel
	log       *internal_logger.Logger
}

func CreateProcessor(es *elasticsearch.Client, dlqCh *amqp.Channel, log *internal_logger.Logger) *Processor {
	p := &Processor{
		Es:        es,
		Validator: validator.New(),
		Dlq:       dlqCh,
		log:       log,
	}

	_, err := dlqCh.QueueDeclare("user_dlq", true, false, false, false, nil)
	if err != nil {
		p.log.Error("Failed to declare DLQ", err)
	}

	return p
}

func ProcessUserObject(p *Processor, body []byte) error {
	var uo gen.UserConfirmed

	if err := xml.Unmarshal(body, &uo); err != nil {
		return sendToDLQ(p.Dlq, p.log, body, fmt.Sprintf("unmarshal error: %v", err))
	}

	if err := p.Validator.Struct(uo); err != nil {
		return sendToDLQ(p.Dlq, p.log, body, fmt.Sprintf("validation error: %v", err))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := indexUser(p.Es, ctx, &uo); err != nil {
		return sendToDLQ(p.Dlq, p.log, body, fmt.Sprintf("index error: %v", err))
	}

	p.log.Info("Indexed User", internal_logger.String("user_id", string(uo.Id)))
	return nil
}

func sendToDLQ(dlq *amqp.Channel, log *internal_logger.Logger, body []byte, reason string) error {
	err := dlq.PublishWithContext(
		context.Background(),
		"",
		"user_dlq",
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
		log.Error("Failed to send to DLQ", err)
	}

	return err
}
