package heartbeat

import (
	"context"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

func ConsumeHeartbeats(p *Processor, msgs <-chan amqp.Delivery, ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Println("Heartbeat consumer shutting down...")
			return

		case msg, ok := <-msgs:
			if !ok {
				return
			}

			err := processHeartbeat(p, msg.Body)
			if err != nil {
				msg.Nack(false, false)
			} else {
				msg.Ack(false)
			}
		}
	}
}
