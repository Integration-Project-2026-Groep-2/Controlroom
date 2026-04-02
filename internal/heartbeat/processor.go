package heartbeat

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

type DLQPublisher interface {
	PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error
}

type Processor struct {
	es        *elasticsearch.Client
	validator *validator.Validate
	dlq       DLQPublisher
	log       *internal_logger.Logger
}

func NewProcessor(es *elasticsearch.Client, dlq DLQPublisher, log *internal_logger.Logger) *Processor {
	return &Processor{
		es:        es,
		validator: validator.New(),
		dlq:       dlq,
		log:       log,
	}
}

func CreateProcessor(es *elasticsearch.Client, dlqCh *amqp.Channel, log *internal_logger.Logger) *Processor {
	p := &Processor{
		es:        es,
		validator: validator.New(),
		dlq:       dlqCh,
		log:       log,
	}

	_, err := dlqCh.QueueDeclare("heartbeat_dlq", true, false, false, false, nil)
	if err != nil {
		p.log.Error("Failed to declare DLQ", err)
	}

	return p
}

func ProcessHeartbeat(p *Processor, body []byte) error {
	var hb gen.Heartbeat

	if err := xml.Unmarshal(body, &hb); err != nil {
		sendToDLQ(p.dlq, p.log, body, fmt.Sprintf("unmarshal error: %v", err))
		return err
	}

	if err := p.validator.Struct(hb); err != nil {
		sendToDLQ(p.dlq, p.log, body, fmt.Sprintf("validation error: %v", err))
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := indexHeartbeat(p.es, ctx, &hb); err != nil {
		sendToDLQ(p.dlq, p.log, body, fmt.Sprintf("index error: %v", err))
		return err
	}

	p.log.Info("Indexed heartbeat", internal_logger.String("service_id", hb.ServiceId))
	return nil
}

func sendToDLQ(dlq DLQPublisher, log *internal_logger.Logger, body []byte, reason string) error {
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
		log.Error("Failed to send to DLQ", err)
	}

	return err
}
