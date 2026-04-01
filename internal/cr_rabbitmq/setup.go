package cr_rabbitmq

import (
	"fmt"
	"strings"

	amqp "github.com/rabbitmq/amqp091-go"
)

type InternalRabbitMQ struct {
	Conn  *amqp.Connection
	Chans map[string]*amqp.Channel
}

type ConsumerInfo struct {
	Name      string
	Consumer  string
	AutoAck   bool
	Exclusive bool
	NoLocal   bool
	NoWait    bool
	Args      amqp.Table
}

type Exchange struct {
	Name       string
	Kind       string
	Durable    bool
	AutoDelete bool
	Internal   bool
	NoWait     bool
	Args       amqp.Table
}

type Queue struct {
	Name       string
	Durable    bool
	AutoDelete bool
	Exclusive  bool
	NoWait     bool
	Args       amqp.Table
}

type BindInfo struct {
	Key    string
	NoWait bool
	Args   amqp.Table
}

type Qos struct {
	PrefetchCount int
	PrefetchSize  int
	Global        bool
}

func SetupConsumer(ir *InternalRabbitMQ, cons *ConsumerInfo, ex *Exchange, queue *Queue, bind *BindInfo, qos *Qos) (<-chan amqp.Delivery, error) {
	wrap := func(err error) error {
		// Simple code to make the first letter a capital
		// Source: https://stackoverflow.com/questions/70206380/how-to-capitalize-the-first-letter-of-a-string
		return fmt.Errorf("Setup%sconsumer: %w", (strings.ToUpper(cons.Name[:1]) + cons.Name[1:]), err)
	}

	ch, err := ir.Conn.Channel()
	if err != nil {
		return nil, wrap(err)
	}

	if err = ch.ExchangeDeclare(ex.Name, ex.Kind, ex.Durable, ex.AutoDelete, ex.Internal, ex.NoWait, ex.Args); err != nil {
		return nil, wrap(err)
	}

	q, err := ch.QueueDeclare(queue.Name, queue.Durable, queue.AutoDelete, queue.Exclusive, queue.NoWait, queue.Args)
	if err != nil {
		return nil, wrap(err)
	}

	if err = ch.QueueBind(q.Name, bind.Key, ex.Name, bind.NoWait, bind.Args); err != nil {
		return nil, wrap(err)
	}

	if err = ch.Qos(qos.PrefetchCount, qos.PrefetchSize, qos.Global); err != nil {
		return nil, wrap(err)
	}

	msgs, err := ch.Consume(q.Name, cons.Consumer, cons.AutoAck, cons.Exclusive, cons.NoLocal, cons.NoWait, cons.Args)
	if err != nil {
		return nil, wrap(err)
	}

	ir.Chans[cons.Name] = ch
	return msgs, nil
}
