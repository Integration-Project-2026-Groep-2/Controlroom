package userobject

import (
	"context"

	amqp "github.com/rabbitmq/amqp091-go"

	internal_logger "integration-project-ehb/controlroom/pkg/logger"
)

func ConsumeUserObjects(p *Processor, msgs <-chan amqp.Delivery, ctx context.Context, log *internal_logger.Logger) {
	for {
		select {
		case <-ctx.Done():
			log.Info("UserObject consumer shutting down")
			return

		case msg, ok := <-msgs:
			if !ok {
				log.Warn("User consumer channel closed unexpectedly")
				return
			}

			err := ProcessUserObject(p, msg.Body)
			if err != nil {
				msg.Nack(false, false)
			} else {
				msg.Ack(false)
			}
		}
	}
}
