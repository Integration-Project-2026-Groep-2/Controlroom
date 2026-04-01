package userobject

import (
	"context"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

func ConsumeCompanies(p *Processor, msgs <-chan amqp.Delivery, ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Println("Company consumer shutting down...")
			return

		case msg, ok := <-msgs:
			if !ok {
				// NOTE(nasr): dont exit just like this????
				// just crashing the program for no reason :(
				// log.Fatalf("User consumer Failed.")
				log.Printf("User consumer Failed.")
				return
			}

			err := ProcessCompanies(p, msg.Body)
			if err != nil {
				msg.Nack(false, false)
			} else {
				msg.Ack(false)
			}
		}
	}
}
