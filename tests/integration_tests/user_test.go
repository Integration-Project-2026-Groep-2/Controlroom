package integration_tests

import (
	"context"
	"encoding/xml"
	"integration-project-ehb/controlroom/internal/cr_config"
	"integration-project-ehb/controlroom/internal/cr_rabbitmq"
	"integration-project-ehb/controlroom/pkg/gen"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	amqp "github.com/rabbitmq/amqp091-go"
)

var adminUser = gen.UserConfirmed{
	Id:          gen.UUIDType("550e8400-e29b-41d4-a716-446655440000"),
	Email:       gen.EmailType("admin@event-platform.com"),
	FirstName:   "Jane",
	LastName:    "Doe",
	IsActive:    true,
	GdprConsent: true,
	Role:        gen.UserRoleTypeADMIN,
	ConfirmedAt: gen.ISO8601DateTimeType(time.Now().Format(time.RFC3339)),
}
var speakerUser = gen.UserConfirmed{
	Id:          gen.UUIDType("a3b8c9d0-1234-5678-90ab-cdef12345678"),
	Email:       gen.EmailType("tech.speaker@partner.com"),
	FirstName:   "Alex",
	LastName:    "Smith",
	IsActive:    true,
	GdprConsent: true,
	Role:        gen.UserRoleTypeSPEAKER,
	ConfirmedAt: gen.ISO8601DateTimeType("2026-03-28T13:45:00Z"),
}

// heartbeat configuation copied from "integration-project/controlroom/cmd/config.go"
var UserDefinition = config.ConsumerDef{
	Type:     config.USER,
	Exchange: cr_rabbitmq.ExchangeInfo{Name: "user.topic", Kind: "topic", Durable: true},
	Queue:    cr_rabbitmq.QueueInfo{Name: "crm.user.confirmed", Durable: true},
	Binding:  cr_rabbitmq.BindingInfo{Key: "crm.user.confirmed"},
	Qos:      10,
}

func publishUser(t *testing.T, ch *amqp.Channel, user gen.UserConfirmed, label string) {

	t.Helper()
	validate := validator.New()

	if err := validate.Struct(user); err != nil {
		t.Logf("%s validation failed: %v", label, err)
		return
	}
	xmlData, _ := xml.Marshal(user)
	t.Logf("Publishing %s: %s", label, string(xmlData))
	err := ch.PublishWithContext(context.Background(), UserDefinition.Exchange.Name, UserDefinition.Binding.Key, false, false, amqp.Publishing{
		ContentType: "text/xml",
		Body:        xmlData,
	},
	)
	if err != nil {
		t.Logf("Failed to publish %s: %v", label, err)
		return
	}
	t.Logf("Sent: %s", label)
}

func TestProducerUser(t *testing.T) {
	conn := SetupAMPConnection(t, URL)
	ch := SetupTestsChannel(t, conn)

	if err := ch.ExchangeDeclare(UserDefinition.Exchange.Name, UserDefinition.Exchange.Kind, true, false, false, false, nil); err != nil {
		t.Fatalf("Failed to declare exchange: %v", err)
	}
	q, err := ch.QueueDeclare(UserDefinition.Queue.Name, true, false, false, false, nil)
	if err != nil {
		t.Fatalf("Failed to declare queue: %v", err)
	}
	if err := ch.QueueBind(q.Name, UserDefinition.Binding.Key, UserDefinition.Exchange.Name, false, nil); err != nil {
		t.Fatalf("Failed to bind queue: %v", err)
	}

	publishUser(t, ch, adminUser, "admin user confirmed")
	publishUser(t, ch, speakerUser, "speaker user confirmed")

	time.Sleep(5 * time.Second)
}
