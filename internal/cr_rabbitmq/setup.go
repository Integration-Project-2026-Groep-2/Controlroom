package cr_rabbitmq

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

type consumerInfo struct {
	Name      string
	Consumer  string
	AutoAck   bool
	Exclusive bool
	NoLocal   bool
	NoWait    bool
	Args      amqp.Table
}

type exchange struct {
	Name       string
	Kind       string
	Durable    bool
	AutoDelete bool
	Internal   bool
	NoWait     bool
	Args       amqp.Table
}

type queue struct {
	Name       string
	Durable    bool
	AutoDelete bool
	Exclusive  bool
	NoWait     bool
	Args       amqp.Table
}

type bindInfo struct {
	Key    string
	NoWait bool
	Args   amqp.Table
}

type qos struct {
	PrefetchCount int
	PrefetchSize  int
	Global        bool
}

// SetupDLQ declares the dead letter queue. Call once at startup.
func SetupDLQ(dlqCh *amqp.Channel, dlqName string) error {
	if dlqName == "" {
		dlqName = "dlq"
	}
	_, err := dlqCh.QueueDeclare(dlqName, true, false, false, false, nil)
	return err
}

// setupConsumer TODO(nasr): implement this function in the entry point
func _(ir *internalRabbitMQ, cons *consumerInfo, ex *exchange, queue *queue, bind *bindInfo, qos *qos) (<-chan amqp.Delivery, error) {

	// NOTE(nasr): @Steven nice good catch but we're handling this in the meta program now but good job finding it!!
	// i thought i'd remove the caps because that is the json convention but that was an oopsies i guess
	wrap := func(err error) error {
		return fmt.Errorf("failed to setup user consumer: %w", err)
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
