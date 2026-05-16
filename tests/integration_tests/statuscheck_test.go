package integration_tests

import (
	"context"
	"encoding/xml"
	"testing"
	"time"

	"integration-project-ehb/controlroom/internal/cr_config"
	"integration-project-ehb/controlroom/internal/cr_rabbitmq"
	"integration-project-ehb/controlroom/pkg/gen"

	"github.com/go-playground/validator/v10"
	amqp "github.com/rabbitmq/amqp091-go"
)

var statusCheckFromServices = []gen.StatusCheckDoc{
	{
		ServiceId: "crm",
		Timestamp: time.Now().UTC(),
		Uptime:    23904,
		Memory:    3434.304,
		Disk:      23088.304,
	},
	{
		ServiceId: "planning",
		Timestamp: time.Now().UTC(),
		Uptime:    18234,
		Memory:    2890.12,
		Disk:      19876.44,
	},
	{
		ServiceId: "kassa",
		Timestamp: time.Now().UTC(),
		Uptime:    9054,
		Memory:    1540.89,
		Disk:      12034.11,
	},
	{
		ServiceId: "frontend",
		Timestamp: time.Now().UTC(),
		Uptime:    30567,
		Memory:    2100.45,
		Disk:      8765.22,
	},
	{
		ServiceId: "mailing",
		Timestamp: time.Now().UTC(),
		Uptime:    45678,
		Memory:    980.66,
		Disk:      6543.90,
	},
	{
		ServiceId: "facturatie",
		Timestamp: time.Now().UTC(),
		Uptime:    22345,
		Memory:    2675.33,
		Disk:      14321.77,
	},
}

var StatusCheckDefinition = config.ConsumerDef{

	Type:     config.STATUSCHECK,
	Exchange: cr_rabbitmq.ExchangeInfo{Name: "statuscheck.direct", Kind: "direct", Durable: true},
	Queue:    cr_rabbitmq.QueueInfo{Name: "controlroom.statuscheck.queue", Durable: true},
	Binding:  cr_rabbitmq.BindingInfo{Key: "routing.statuscheck"},
	DLQName:  "controlroom.statuscheck.queue.dlq",
	Qos:      5,
}

func publishStatusCheck(t *testing.T, ch *amqp.Channel, sc gen.StatusCheckDoc, label string) {
	t.Helper()

	validate := validator.New()
	if err := validate.Struct(sc); err != nil {
		t.Fatalf("%s validation failed: %v", label, err)
	}

	xmlData, err := xml.Marshal(sc)
	if err != nil {
		t.Fatalf("%s marshal failed: %v", label, err)
	}

	t.Logf("Publishing %s: %s", label, string(xmlData))

	if err := ch.PublishWithContext(
		context.Background(),
		StatusCheckDefinition.Exchange.Name,
		StatusCheckDefinition.Binding.Key,
		false,
		false,
		amqp.Publishing{
			ContentType: "text/xml",
			Body:        xmlData,
		},
	); err != nil {
		t.Fatalf("failed to publish %s: %v", label, err)
	}

	t.Logf("Sent: %s", label)
}

func TestStatuscheck(t *testing.T) {
	conn := SetupAMPConnection(t, URL)

	pubCh := SetupTestsChannel(t, conn)
	defer pubCh.Close()

	subCh := SetupTestsChannel(t, conn)
	defer subCh.Close()

	if err := pubCh.ExchangeDeclare(StatusCheckDefinition.Exchange.Name, StatusCheckDefinition.Exchange.Kind, true, false, false, false, nil); err != nil {
		t.Fatalf("failed to declare exchange: %v", err)
	}

	q, err := subCh.QueueDeclare(StatusCheckDefinition.Queue.Name, true, false, false, false, nil)
	if err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	if err := subCh.QueueBind(q.Name, StatusCheckDefinition.Binding.Key, StatusCheckDefinition.Exchange.Name, false, nil); err != nil {
		t.Fatalf("failed to bind queue: %v", err)
	}

	// TODO(nasr): do this later, assert this later, see clickup
	// msgs, err := subCh.Consume(q.Name, "", false, false, false, false, nil)

	if err != nil {
		t.Fatalf("failed to start consumer: %v", err)
	}

	for _, service := range statusCheckFromServices {
		publishStatusCheck(t, pubCh, service, "statuscheck")
	}
}
