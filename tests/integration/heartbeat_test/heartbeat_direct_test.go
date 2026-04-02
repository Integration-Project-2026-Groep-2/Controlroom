// testing if the heartbeat lifecycle works
package producer_test

import (
	"context"
	"encoding/xml"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	amqp "github.com/rabbitmq/amqp091-go"

	"integration-project-ehb/controlroom/pkg/gen"
)

const (
	rabbitmqURL      = "amqp://guest:guest@localhost:5672/"
	exchangeName     = "heartbeat.direct"
	routingKey       = "routing.heartbeat"
	testQueue        = "heartbeat_queue"
	roundTripTimeout = 5 * time.Second
)

func setupChannel(t *testing.T, conn *amqp.Connection) *amqp.Channel {
	t.Helper()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("failed to open channel: %v", err)
	}
	return ch
}

func TestHeartbeatRoundTrip(t *testing.T) {
	validate := validator.New()

	conn, err := amqp.Dial(rabbitmqURL)
	if err != nil {
		t.Fatalf("failed to connect to RabbitMQ: %v", err)
	}
	defer conn.Close()

	// Producer channel
	pubCh := setupChannel(t, conn)
	defer pubCh.Close()

	// Consumer channel
	subCh := setupChannel(t, conn)
	defer subCh.Close()

	// Declare exchange
	if err = pubCh.ExchangeDeclare(exchangeName, "direct", true, false, false, false, nil); err != nil {
		t.Fatalf("failed to declare exchange: %v", err)
	}

	// Declare a temporary queue for the test
	q, err := subCh.QueueDeclare(testQueue, true, false, false, false, nil)
	if err != nil {
		t.Fatalf("failed to declare test queue: %v", err)
	}
	defer subCh.QueueDelete(q.Name, false, false, false)

	if err = subCh.QueueBind(q.Name, routingKey, exchangeName, false, nil); err != nil {
		t.Fatalf("failed to bind queue: %v", err)
	}

	msgs, err := subCh.Consume(q.Name, "", false, false, false, false, nil)
	if err != nil {
		t.Fatalf("failed to start consumer: %v", err)
	}

	// Build and validate heartbeat
	hb := gen.Heartbeat{
		ServiceId: "test-service",
		Timestamp: time.Now().UTC(),
	}
	if err = validate.Struct(hb); err != nil {
		t.Fatalf("heartbeat validation failed: %v", err)
	}

	xmlData, err := xml.Marshal(hb)
	if err != nil {
		t.Fatalf("failed to marshal heartbeat: %v", err)
	}

	// Publish
	if err = pubCh.PublishWithContext(
		context.Background(),
		exchangeName,
		routingKey,
		false,
		false,
		amqp.Publishing{
			ContentType: "text/xml",
			Body:        xmlData,
		},
	); err != nil {
		t.Fatalf("failed to publish heartbeat: %v", err)
	}

	// Wait for message with timeout
	select {
	case msg, ok := <-msgs:
		if !ok {
			t.Fatal("consumer channel closed unexpectedly")
		}

		var received gen.Heartbeat
		if err = xml.Unmarshal(msg.Body, &received); err != nil {
			t.Fatalf("failed to unmarshal received message: %v", err)
		}

		if received.ServiceId != hb.ServiceId {
			t.Errorf("ServiceId mismatch: got %q, want %q", received.ServiceId, hb.ServiceId)
		}
		if !received.Timestamp.Equal(hb.Timestamp) {
			t.Errorf("Timestamp mismatch: got %v, want %v", received.Timestamp, hb.Timestamp)
		}

	case <-time.After(roundTripTimeout):
		t.Fatal("timed out waiting for heartbeat message")
	}
}
