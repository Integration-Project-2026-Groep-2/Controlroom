package cr_rabbitmq

import (
	"fmt"
	"os"

	amqp "github.com/rabbitmq/amqp091-go"
)

func SetupHeartbeatConsumer(*amqp.Connection) (*amqp.Channel, <-chan amqp.Delivery, error) {

	conn, err := amqp.Dial(os.Getenv("RABBITMQ_URL"))

	if err != nil {
		return nil, nil, err
	}

	ch, err := conn.Channel()
	if err != nil {
		return nil, nil, fmt.Errorf("failed here")
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
		return nil, nil, err
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
		return nil, nil, err
	}

	err = ch.QueueBind(
		qHeartbeat.Name,
		"routing.heartbeat",
		"control_room_exchange",
		false,
		nil,
	)
	if err != nil {
		return nil, nil, err
	}

	err = ch.Qos(10, 0, false)
	if err != nil {
		return nil, nil, err
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
		return nil, nil, err
	}

	return ch, msgs, nil
}

func SetupUserConsumer(*amqp.Connection) (*amqp.Channel, <-chan amqp.Delivery, error) {
	conn, err := amqp.Dial(os.Getenv("RABBITMQ_URL"))

	if err != nil {
		return nil, nil, err
	}

	ch, err := conn.Channel()
	if err != nil {
		return nil, nil, err
	}

	err = ch.ExchangeDeclare(
		"user.topic",
		"direct",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return nil, nil, err
	}

	Quser, err := ch.QueueDeclare(
		"crm.user.confirmed",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return nil, nil, err
	}

	err = ch.QueueBind(
		Quser.Name,
		// TODO(Steven) Add actual routing key when exists
		"temp.routing.consumers",
		"user.topic",
		false,
		nil,
	)
	if err != nil {
		return nil, nil, err
	}

	err = ch.Qos(10, 0, false)
	if err != nil {
		return nil, nil, err
	}

	msgs, err := ch.Consume(
		Quser.Name,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return nil, nil, err
	}

	return ch, msgs, nil
}
