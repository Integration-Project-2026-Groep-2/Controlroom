package integration_tests

import (
	"log"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	URL = "amqp://guest:guest@localhost:5672/"
)

func SetupAMPConnection(t *testing.T, adress string) *amqp.Connection {

	conn, err := amqp.Dial(URL)

	if err == nil {
		return conn
	}

	conn.Close()
	log.Fatal("Failed to connect to rabbitmq")
	return nil
}

func SetupTestsChannel(t *testing.T, conn *amqp.Connection) *amqp.Channel {
	t.Helper()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("failed to open channel: %v", err)
	}
	return ch
}
