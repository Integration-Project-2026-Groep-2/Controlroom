package cr_rabbitmq

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	amqp "github.com/rabbitmq/amqp091-go"

	"integration-project-ehb/controlroom/pkg/logger"
)

type internalRabbitMQ struct {
	Conn  *amqp.Connection
	Chans map[string]*amqp.Channel
}

// ConsumerConfig holds the configuration for message consumption.
type ConsumerConfig struct {
	client  *elasticsearch.Client
	DLQCh   *amqp.Channel
	DLQName string
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

// SetupConsumer declares an exchange, queue, and binding, then returns a delivery channel.
func SetupConsumer(ch *amqp.Channel, ex ExchangeInfo, q QueueInfo, binding BindingInfo) (<-chan amqp.Delivery, error) {

	if q.Args == nil {
		q.Args = amqp.Table{}
	}
	// Add DLX configuration
	q.Args["x-dead-letter-exchange"] = "controlroom.dlx"
	q.Args["x-dead-letter-routing-key"] = q.Name + ".failed"

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

	consumerTag := fmt.Sprintf("controlroom-%d", os.Getpid())

	msgs, err := ch.Consume(queue.Name, consumerTag, false, false, false, false, nil)
	if err != nil {
		return nil, err
	}

	return msgs, nil
}

// SendToDLQ publishes a message to the dead letter queue with error context.
func SendToDLQ(dlqCh *amqp.Channel, dlqName string, body []byte, reason string, exchange string) error {

	if dlqName == "" {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, "Wrong deadletter parameter passed"))
	}

	key := strings.Join([]string{exchange, ".failed"}, ".")

	return dlqCh.PublishWithContext(
		context.Background(),
		"controlroom.dlx", // exchange
		key,               // routing key
		false,             // mandatory
		false,             // immediate
		amqp.Publishing{
			ContentType: "application/octet-stream",
			Body:        body,
			Headers: amqp.Table{
				"error_reason":              reason,
				"timestamp":                 time.Now().Unix(),
				"x-dead-letter-exchange":    "controlroom.dlx",
				"x-dead-letter-routing-key": key,
			},
		},
	)
}

// Consume reads from a delivery channel and processes each message.
// On success, acks the message. On error, sends to DLQ and nacks.
// Blocks until ctx is cancelled.
func Consume(cfg *ConsumerConfig, msgs <-chan amqp.Delivery, ctx context.Context, handler func(*elasticsearch.Client, []byte) error) error {
	for {
		select {
		case <-ctx.Done():
			logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, "Consumer shutting down..."))
			return nil

		case msg, ok := <-msgs:
			if !ok {
				return nil
			}

			err := handler(cfg.client, msg.Body)
			if err != nil {
				logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, fmt.Sprintf("Process failed: %v", err)))

				if err := SendToDLQ(cfg.DLQCh, cfg.DLQName, msg.Body, err.Error(), ""); err != nil {
					logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, fmt.Sprintf("Failed to send to DLQ: %v", err)))
					return err
				}

				err := msg.Nack(false, false)
				if err != nil {
					logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, fmt.Sprintf("Error NACK: %v", err)))
					return err
				}

			} else {

				err := msg.Ack(false)
				if err != nil {
					logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, fmt.Sprintf("Error ACK: %v", err)))
					return err
				}
			}
		}
	}
}
