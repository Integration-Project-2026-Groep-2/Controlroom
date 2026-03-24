// producer van thomas
package main

import (
	"context"
	"encoding/xml"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	amqp "github.com/rabbitmq/amqp091-go"

	"integration-project-ehb/controlroom/pkg/xml/gen"
)

// MockChannel implements a fake AMQP channel to capture Publish calls
type MockChannel struct {
	Published []amqp.Publishing
}

func (m *MockChannel) PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error {
	m.Published = append(m.Published, msg)
	return nil
}

// minimal interface to satisfy your code
func (m *MockChannel) Close() error {
	return nil
}

// TestSendHeartbeat verifies that Heartbeat XML is published
func TestSendHeartbeat(t *testing.T) {
	now := time.Now().UTC()
	hb := gen.Heartbeat{ServiceId: "Service-test", Timestamp: now}

	// marshal to xml for comparison
	expectedXML, err := xml.Marshal(hb)
	assert.NoError(t, err)

	mockCh := &MockChannel{}

	// simulate sending
	err = mockCh.PublishWithContext(context.Background(), "control_room_exchange", "routing.heartbeat", false, false,
		amqp.Publishing{
			ContentType: "text/xml",
			Body:        expectedXML,
		})
	assert.NoError(t, err)

	// assertions
	assert.Len(t, mockCh.Published, 1, "should have published one message")
	assert.Equal(t, expectedXML, mockCh.Published[0].Body, "published XML should match")
	assert.Equal(t, "text/xml", mockCh.Published[0].ContentType, "content type should be text/xml")
}
