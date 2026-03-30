package cr_rabbitmq

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

type InternalRabbitMQ struct {
	Conn  *amqp.Connection
	Chans map[string]*amqp.Channel
}

func SetupHeartbeatConsumer(ir *InternalRabbitMQ) (<-chan amqp.Delivery, error) {
	// NOTE(nasr): code cleanup
	wrap := func(err error) error {
		return fmt.Errorf("SetupHeartbeatConsumer: %w", err)
	}

	ch, err := ir.Conn.Channel()
	if err != nil {
		// TODO(nasr): replace with logging
		return nil, wrap(err)
	}

	if err = ch.ExchangeDeclare("control_room_exchange", "direct", true, false, false, false, nil); err != nil {
		return nil, wrap(err)
	}
	if err = ch.ExchangeDeclare("heartbeat.direct", "direct", true, false, false, false, nil); err != nil {
		return nil, wrap(err)
	}

	q, err := ch.QueueDeclare("heartbeat_queue", false, true, false, false, nil)
	if err != nil {
		return nil, wrap(err)
	}

	if err = ch.QueueBind(q.Name, "routing.heartbeat", "control_room_exchange", false, nil); err != nil {
		return nil, wrap(err)
	}
	if err = ch.QueueBind(q.Name, "routing.heartbeat", "heartbeat.direct", false, nil); err != nil {
		return nil, wrap(err)
	}

	// NOTE(nasr): prefetch count, size and if application is true or false
	// set to 6 because 6 microservices?

	if err = ch.Qos(6, 0, true); err != nil {
		return nil, wrap(err)
	}

	msgs, err := ch.Consume(q.Name, "", false, false, false, false, nil)
	if err != nil {
		return nil, wrap(err)
	}

	ir.Chans["heartbeat"] = ch
	return msgs, nil
}

func SetupUserConsumer(ir *InternalRabbitMQ) (<-chan amqp.Delivery, error) {
	wrap := func(err error) error {
		return fmt.Errorf("SetupUserConsumer: %w", err)
	}

	ch, err := ir.Conn.Channel()
	if err != nil {
		return nil, wrap(err)
	}

	if err = ch.ExchangeDeclare("user.topic", "direct", true, false, false, false, nil); err != nil {
		return nil, wrap(err)
	}
	if err = ch.ExchangeDeclare("heartbeat.direct", "direct", true, false, false, false, nil); err != nil {
		return nil, wrap(err)
	}

	q, err := ch.QueueDeclare("crm.user.confirmed", true, false, false, false, nil)
	if err != nil {
		return nil, wrap(err)
	}

	if err = ch.QueueBind(q.Name,
		// TODO(Steven): Add actual routing key when exists
		"temp.routing.consumers", "user.topic", false, nil); err != nil {
		return nil, wrap(err)
	}
	if err = ch.QueueBind(q.Name, "routing.user.confirmed", "heartbeat.direct", false, nil); err != nil {
		return nil, wrap(err)
	}

	// NOTE(nasr): prefetch count, size and if application is true or false
	if err = ch.Qos(10, 0, false); err != nil {
		return nil, wrap(err)
	}

	msgs, err := ch.Consume(q.Name, "", false, false, false, false, nil)
	if err != nil {
		return nil, wrap(err)
	}

	ir.Chans["user"] = ch
	return msgs, nil
}
