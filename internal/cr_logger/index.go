package cr_logger

import (
	"context"
	"fmt"
	amqp "github.com/rabbitmq/amqp091-go"
	"integration-project-ehb/controlroom/internal/cr_rabbitmq"
)

func ConsumeLogs(ctx context.Context, ch *amqp.Channel, dlqCh *amqp.Channel) error {
	msgs, err := cr_rabbitmq.SetupQueue(
		ch,
		cr_rabbitmq.ExchangeInfo{
			Name:       "logs.direct",
			Kind:       "direct",
			Durable:    true,
			AutoDelete: false,
			Internal:   false,
			NoWait:     false,
			Args:       nil,
		},

		cr_rabbitmq.QueueInfo{
			Name:       "logs.queue",
			Durable:    true,
			AutoDelete: false,
			Exclusive:  false,
			NoWait:     false,
			Args:       nil,
		},

		cr_rabbitmq.BindingInfo{
			Key:    "log",
			NoWait: false,
			Args:   nil,
		},
	)
	if err != nil {
		return fmt.Errorf("SetupLogsConsumer: %w", err)
	}

	cfg := &cr_rabbitmq.ConsumerConfig{
		DLQCh:   dlqCh,
		DLQName: "dlq",
		Process: LogMessageProcesser,
	}

	go cr_rabbitmq.Consume(cfg, msgs, ctx)
	return nil
}
