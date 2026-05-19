package cr_rabbitmq

import (
	"bytes"
	"encoding/xml"
	"integration-project-ehb/controlroom/pkg/gen"
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

	return channel.Publish("health.exchange", "heartbeat", false, false,
		amqp.Publishing{
			ContentType: "application/xml",
			Timestamp:   time.Now().UTC(),
			Body:        buf.Bytes(),
		},
	)
}
