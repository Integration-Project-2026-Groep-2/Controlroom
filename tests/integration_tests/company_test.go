package integration_tests

import (
	"context"
	"encoding/xml"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	amqp "github.com/rabbitmq/amqp091-go"

	"integration-project-ehb/controlroom/cmd/config"
	"integration-project-ehb/controlroom/internal/cr_rabbitmq"
	"integration-project-ehb/controlroom/pkg/gen"
)

var CompanyDefinition = config.ConsumerDef{
	Type:     config.COMPANY,
	Exchange: cr_rabbitmq.ExchangeInfo{Name: "contact.topic", Kind: "topic", Durable: true},
	Queue:    cr_rabbitmq.QueueInfo{Name: "crm.company.confirmed", Durable: true},
	Binding:  cr_rabbitmq.BindingInfo{Key: "crm.company.confirmed"},
	Qos:      10,
	Passive:  true,
}

var ActiveCompany = gen.CompanyConfirmed{
	Id:          gen.UUIDType("550e8400-e29b-41d4-a716-446655440000"),
	VatNumber:   gen.BelgianVatNumberType("BE0123456789"),
	Name:        "Doe Industries",
	Email:       gen.EmailType("contact@doe-industries.com"),
	Street:      "Keizerslaan",
	HouseNumber: "11",
	PostalCode:  "1000",
	City:        "Brussel",
	Country:     gen.CountryCodeType("BE"),
	IsActive:    true,
	ConfirmedAt: gen.ISO8601DateTimeType(time.Now().UTC().Format(time.RFC3339)),
}

func publishCompany(t *testing.T, ch *amqp.Channel, company gen.CompanyConfirmed, label string) {

	t.Helper()
	validate := validator.New()

	if err := validate.Struct(company); err != nil {
		t.Logf("Company validation failed")
		return
	}

	data, _ := xml.Marshal(company)

	t.Logf("Channel status check, marshalled body: %s", string(data))

	err := ch.PublishWithContext(
		context.Background(),
		CompanyDefinition.Exchange.Name,
		CompanyDefinition.Binding.Key,
		false,
		false,
		amqp.Publishing{
			ContentType: "text/xml",
			Body:        data,
		},
	)

	if err != nil {
		t.Logf("Failed to publish %s: %v", label, err)
		return
	}

	t.Logf("Sent: %s", label)

}

func TestProducerCompany(t *testing.T) {

	conn := SetupAMPConnection(t, URL)
	ch := SetupTestsChannel(t, conn)

	if err := ch.ExchangeDeclare(CompanyDefinition.Exchange.Name, CompanyDefinition.Exchange.Kind, true, false, false, false, nil); err != nil {
		t.Fatalf("Failed to declare exchange: %v", err)
	}
	q, err := ch.QueueDeclare(CompanyDefinition.Queue.Name, true, false, false, false, nil)
	if err != nil {
		t.Fatalf("Failed to declare queue: %v", err)
	}
	if err := ch.QueueBind(q.Name, CompanyDefinition.Binding.Key, CompanyDefinition.Exchange.Name, false, nil); err != nil {
		t.Fatalf("Failed to bind queue: %v", err)
	}

	publishCompany(t, ch, ActiveCompany, "admin user confirmed")
	//	publishCompany(t, ch, InActiveCompany, "speaker user confirmed")

	time.Sleep(5 * time.Second)

}
