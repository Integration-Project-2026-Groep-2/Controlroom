package userobject

import (
	"context"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

func ConsumeUserObjects(p *Processor, msgs <-chan amqp.Delivery, ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Println("UserObject consumer shutting down...")
			return

		case msg, ok := <-msgs:
			if !ok {
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
