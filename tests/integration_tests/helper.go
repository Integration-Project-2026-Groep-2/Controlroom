package integration_tests

import (
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	URL = "amqp://guest:guest@localhost:5672/"
)

func SetupAMPConnection(t *testing.T, adress string) *amqp.Connection {
	t.Helper()

	conn, err := amqp.Dial(adress)
	if err != nil {
		t.Fatalf("failed to connect to rabbitmq: %v", err)
	}

	return conn
}

func SetupTestsChannel(t *testing.T, conn *amqp.Connection) *amqp.Channel {
	t.Helper()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("failed to open channel: %v", err)
	}
	return ch
}
