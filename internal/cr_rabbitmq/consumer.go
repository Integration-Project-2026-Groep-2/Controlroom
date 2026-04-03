package cr_rabbitmq

import (
	"context"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"integration-project-ehb/controlroom/pkg/logger"
)

type InternalRabbitMQ struct {
	Conn  *amqp.Connection
	Chans map[string]*amqp.Channel
}

// ProcessFunc is a function type that processes a message body.
// Return nil on success, or an error to trigger DLQ routing.
type ProcessFunc func(body []byte) error

// ConsumerConfig holds the configuration for message consumption.
type ConsumerConfig struct {
	DLQCh   *amqp.Channel
	DLQName string
	Process ProcessFunc
	Log     *logger.Logger
}

// ExchangeInfo holds exchange declaration parameters.
type ExchangeInfo struct {
	Name       string
	Kind       string
	Durable    bool
	AutoDelete bool
	Internal   bool
	NoWait     bool
	Args       amqp.Table
}

// QueueInfo holds queue declaration parameters.
type QueueInfo struct {
	Name       string
	Durable    bool
	AutoDelete bool
	Exclusive  bool
	NoWait     bool
	Args       amqp.Table
}

// BindingInfo holds binding parameters.
type BindingInfo struct {
	Key    string
	NoWait bool
	Args   amqp.Table
}

// SetupQueue declares an exchange, queue, and binding, then returns a delivery channel.
func SetupQueue(ch *amqp.Channel, ex ExchangeInfo, q QueueInfo, binding BindingInfo) (<-chan amqp.Delivery, error) {
	if err := ch.ExchangeDeclare(ex.Name, ex.Kind, ex.Durable, ex.AutoDelete, ex.Internal, ex.NoWait, ex.Args); err != nil {
		return nil, err
	}

	queue, err := ch.QueueDeclare(q.Name, q.Durable, q.AutoDelete, q.Exclusive, q.NoWait, q.Args)
	if err != nil {
		return nil, err
	}

	if err := ch.QueueBind(queue.Name, binding.Key, ex.Name, binding.NoWait, binding.Args); err != nil {
		return nil, err
	}

	msgs, err := ch.Consume(queue.Name, "", false, false, false, false, nil)
	if err != nil {
		return nil, err
	}

	return msgs, nil
}

// SendToDLQ publishes a message to the dead letter queue with error context.
func SendToDLQ(dlqCh *amqp.Channel, dlqName string, body []byte, reason string) error {
	if dlqName == "" {
		dlqName = "dlq"
	}
	return dlqCh.PublishWithContext(
		context.Background(),
		"",      // exchange
		dlqName, // routing key
		false,   // mandatory
		false,   // immediate
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

// Consume reads from a delivery channel and processes each message.
// On success, acks the message. On error, sends to DLQ and nacks.
// Blocks until ctx is cancelled.
func Consume(cfg *ConsumerConfig, msgs <-chan amqp.Delivery, ctx context.Context) {
	if cfg.DLQName == "" {
		cfg.DLQName = "dlq"
	}

	log := cfg.Log

	for {
		select {
		case <-ctx.Done():
			log.Info("consumer shutting down")
			return
		case msg, ok := <-msgs:
			if !ok {
				return
			}
			err := cfg.Process(msg.Body)
			if err != nil {
				log.Error("process failed", err)
				// Send to DLQ before nack
				if dlqErr := SendToDLQ(cfg.DLQCh, cfg.DLQName, msg.Body, err.Error()); dlqErr != nil {
					log.Error("failed to send to DLQ", dlqErr)
				}
				msg.Nack(false, false)
			} else {
				msg.Ack(false)
			}
		}
	}
}
