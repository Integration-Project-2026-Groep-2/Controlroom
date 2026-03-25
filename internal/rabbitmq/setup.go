package cr_rabbitmq

import (
	"fmt"
	amqp "github.com/rabbitmq/amqp091-go"
	"log"
)

func SetupHeartbeatConsumer() (*amqp.Connection, *amqp.Channel, <-chan amqp.Delivery, error) {

	// TODO(marwan): replace with secrets
	conn, err := amqp.Dial("amqp://guest:guest@rabbitmq:5672/")

	log.Printf("connection value: %v", conn)

	if err != nil {
		return nil, nil, nil, err
	}

	ch, err := conn.Channel()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed here")
	}

	err = ch.ExchangeDeclare(
		"control_room_exchange",
		"direct",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return nil, nil, nil, err
	}

	qHeartbeat, err := ch.QueueDeclare(
		"heartbeat_queue",
		false,
		true,
		false,
		false,
		nil,
	)
	if err != nil {
		return nil, nil, nil, err
	}

	err = ch.QueueBind(
		qHeartbeat.Name,
		"routing.heartbeat",
		"control_room_exchange",
		false,
		nil,
	)
	if err != nil {
		return nil, nil, nil, err
	}

	err = ch.Qos(10, 0, false)
	if err != nil {
		return nil, nil, nil, err
	}

	msgs, err := ch.Consume(
		qHeartbeat.Name,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return nil, nil, nil, err
	}

	return conn, ch, msgs, nil
}
