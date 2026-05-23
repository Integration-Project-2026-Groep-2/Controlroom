package rmq_heartbeat_publisher

import (
	"bytes"
	"context"
	"encoding/xml"
	"integration-project-ehb/controlroom/pkg/gen"

	"integration-project-ehb/controlroom/internal/cr_config"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// this acts as the heartbeat for testing rabbitmq
func PublishHeartbeat(channel *amqp.Channel) error {

	hb := gen.Heartbeat{
		ServiceId: "RabbitMQ",
		Timestamp: time.Now().UTC(),
	}

	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")

	if err := enc.Encode(hb); err != nil {
		return err
	}

	if err := enc.Flush(); err != nil {
		return err
	}

	err := channel.PublishWithContext(
		context.Background(),
		config.Producer[config.RMQ_HEARTBEAT].Exchange.Name,
		config.Producer[config.RMQ_HEARTBEAT].Key.Key,
		false, false,
		amqp.Publishing{
			ContentType: "application/xml",
			Timestamp:   time.Now().UTC(),
			Body:        buf.Bytes(),
		},
	)

	if err != nil {
		return err

	}

	return nil
}
