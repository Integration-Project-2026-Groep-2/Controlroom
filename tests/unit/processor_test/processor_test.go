// Package processor_test test de kernlogica van de heartbeat processor.
//
// Test 1 (InvalidXML): stuurt kapotte XML → verwacht dat het bericht naar de DLQ gaat.
// Test 2 (ValidXML_SendsToDLQOnESError): stuurt geldige XML maar ES is niet bereikbaar → verwacht DLQ.
//
// mockDLQ vervangt een echte RabbitMQ verbinding zodat tests zonder infrastructuur draaien.
package main

import (
	"context"
	"encoding/xml"
	"io"
	"testing"
	"time"

	elasticsearch "github.com/elastic/go-elasticsearch/v9"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"

	"integration-project-ehb/controlroom/internal/heartbeat"
	internal_logger "integration-project-ehb/controlroom/pkg/logger"
	"integration-project-ehb/controlroom/pkg/xml/gen"
)

type mockDLQ struct {
	called bool
	reason string
}

func (m *mockDLQ) PublishWithContext(_ context.Context, _, _ string, _, _ bool, msg amqp.Publishing) error {
	m.called = true
	if r, ok := msg.Headers["error_reason"]; ok {
		m.reason = r.(string)
	}
	return nil
}

func newTestProcessor(dlq heartbeat.DLQPublisher) *heartbeat.Processor {
	es, _ := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{"http://localhost:9999"}, // nep URL zodat ES altijd faalt
	})
	return heartbeat.NewProcessor(es, dlq, internal_logger.New(io.Discard, "test"))
}

func TestProcessHeartbeat_InvalidXML(t *testing.T) {
	dlq := &mockDLQ{}
	p := newTestProcessor(dlq)

	err := heartbeat.ProcessHeartbeat(p, []byte("niet geldig xml"))

	assert.Error(t, err)
	assert.True(t, dlq.called, "DLQ moet aangeroepen worden bij ongeldige XML")
}

func TestProcessHeartbeat_ValidXML_SendsToDLQOnESError(t *testing.T) {
	dlq := &mockDLQ{}
	p := newTestProcessor(dlq)

	hb := gen.Heartbeat{ServiceId: "test-service", Timestamp: time.Now().UTC()}
	body, _ := xml.Marshal(hb)

	// ES client heeft geen echte verbinding, dus indexing faalt → DLQ
	err := heartbeat.ProcessHeartbeat(p, body)

	assert.Error(t, err)
	assert.True(t, dlq.called, "DLQ moet aangeroepen worden als ES niet bereikbaar is")
}
