package heartbeat

import (
	"context"

	amqp "github.com/rabbitmq/amqp091-go"

	internal_logger "integration-project-ehb/controlroom/pkg/logger"
)

func ConsumeHeartbeats(p *Processor, msgs <-chan amqp.Delivery, ctx context.Context, log *internal_logger.Logger) {
	for {
		select {
		case <-ctx.Done():
			log.Info("Heartbeat consumer shutting down")
			return

		case msg, ok := <-msgs:
			if !ok {
				return
			}

			err := ProcessHeartbeat(p, msg.Body)
			if err != nil {
				msg.Nack(false, false)
			} else {
				msg.Ack(false)
			}
		}
	}
}
