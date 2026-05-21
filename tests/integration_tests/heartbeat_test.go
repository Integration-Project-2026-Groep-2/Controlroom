// / testing if the heartbeat  works
package integration_tests

import (
	"context"
	"encoding/xml"
	"sync"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	amqp "github.com/rabbitmq/amqp091-go"

	"integration-project-ehb/controlroom/internal/cr_config"
	"integration-project-ehb/controlroom/internal/cr_rabbitmq"
	"integration-project-ehb/controlroom/pkg/gen"
)

var heartbeatServices = []gen.HeartbeatDoc{
	{
		ServiceId: "crm",
		Timestamp: time.Now().UTC(),
		Indexed:   time.Now().UTC(),
	},
	{
		ServiceId: "planning",
		Timestamp: time.Now().UTC(),
		Indexed:   time.Now().UTC(),
	},
	{
		ServiceId: "facturatie",
		Timestamp: time.Now().UTC(),
		Indexed:   time.Now().UTC(),
	},
	{
		ServiceId: "mailing",
		Timestamp: time.Now().UTC(),
		Indexed:   time.Now().UTC(),
	},
	{
		ServiceId: "frontend",
		Timestamp: time.Now().UTC(),
		Indexed:   time.Now().UTC(),
	},
	{
		ServiceId: "chackamaka",
		Timestamp: time.Now().UTC(),
		Indexed:   time.Now().UTC(),
	},
}

// heartbeat configuation copied from "integration-project/controlroom/cmd/config.go"
var HeartbeatDefinition = config.ConsumerDef{

	Type:     config.HEARTBEAT,
	Exchange: cr_rabbitmq.ExchangeInfo{Name: "heartbeat.direct", Kind: "direct", Durable: true},
	Queue:    cr_rabbitmq.QueueInfo{Name: "controlroom.heartbeat.queue", Durable: true},
	Binding:  cr_rabbitmq.BindingInfo{Key: "routing.heartbeat"},
	DLQName:  "controlroom.heartbeat.queue.dlq",
	Qos:      18,
}

func publishHeartbeat(t *testing.T, ch *amqp.Channel, heartbeat gen.HeartbeatDoc, label string) {

	t.Helper()
	validate := validator.New()

	if err := validate.Struct(heartbeat); err != nil {
		t.Logf("heartbeat validation failed")
		return
	}

	data, _ := xml.Marshal(heartbeat)

	err := ch.PublishWithContext(
		context.Background(),
		HeartbeatDefinition.Exchange.Name,
		HeartbeatDefinition.Binding.Key,
		false, false, amqp.Publishing{
			ContentType: "text/xml",
			Body:        data,
		})

	if err != nil {
		t.Logf("Failed to publish %s: %v", label, err)
		return
	}

	t.Logf("Sent: %s", label)
}

// TestProducerHeartbeat send a single heartbeat from all of the listed services (CRM, Planning, ...)
func TestProducerHeartbeat(t *testing.T) {

	conn := SetupAMPConnection(t, URL)
	ch := SetupTestsChannel(t, conn)

	if err := ch.ExchangeDeclare(HeartbeatDefinition.Exchange.Name, HeartbeatDefinition.Exchange.Kind, true, false, false, false, nil); err != nil {
		t.Fatalf("Failed to declare queue: %v", err)
	}

	q, err := ch.QueueDeclare(HeartbeatDefinition.Queue.Name, true, false, false, false, nil)
	if err != nil {
		t.Fatalf("Failed to declare queue: %v", err)
	}
	if err := ch.QueueBind(q.Name, HeartbeatDefinition.Binding.Key, HeartbeatDefinition.Exchange.Name, false, nil); err != nil {
		t.Fatalf("Failed to bind queue: %v", err)
	}

	for _, service := range heartbeatServices {
		publishHeartbeat(t, ch, service, "Sent Heartbeat")
	}
}

// start 10 go routines that send heaartbs concurrently from all of the simlulated services
func TestProducerMultiple(t *testing.T) {

	conn := SetupAMPConnection(t, URL)
	ch := SetupTestsChannel(t, conn)

	if err := ch.ExchangeDeclare(HeartbeatDefinition.Exchange.Name, HeartbeatDefinition.Exchange.Kind, true, false, false, false, nil); err != nil {
		t.Fatalf("Failed to declare queue: %v", err)
	}

	q, err := ch.QueueDeclare(HeartbeatDefinition.Queue.Name, true, false, false, false, nil)
	if err != nil {
		t.Fatalf("Failed to declare queue: %v", err)
	}
	if err := ch.QueueBind(q.Name, HeartbeatDefinition.Binding.Key, HeartbeatDefinition.Exchange.Name, false, nil); err != nil {
		t.Fatalf("Failed to bind queue: %v", err)
	}

	var wg sync.WaitGroup
	for _, service := range heartbeatServices {
		wg.Go(func() {
			for range 10 {
				publishHeartbeat(t, ch, service, "Sent Heartbeat")
			}
		})
	}
	wg.Wait()
}
