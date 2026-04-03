package heartbeat_test

import (
	"encoding/xml"
	"io"
	"testing"
	"time"

	elasticsearch "github.com/elastic/go-elasticsearch/v9"
	"github.com/stretchr/testify/assert"
	"integration-project-ehb/controlroom/internal/heartbeat"
	"integration-project-ehb/controlroom/pkg/gen"
	"integration-project-ehb/controlroom/pkg/logger"
)

// TestProcessHeartbeat_InvalidXML: malformed XML should fail during unmarshal
func TestProcessHeartbeat_InvalidXML(t *testing.T) {
	// Create ES client (pointing to unreachable endpoint)
	es, _ := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{"http://localhost:9999"},
	})
	processor := heartbeat.NewHeartbeatProcessor(es, logger.New(io.Discard, "test"))

	err := processor([]byte("invalid xml"))
	assert.Error(t, err, "should fail on malformed XML")
	assert.Contains(t, err.Error(), "unmarshal")
}

// TestProcessHeartbeat_ValidXML_ESUnavailable: valid XML but no ES → index error
func TestProcessHeartbeat_ValidXML_ESUnavailable(t *testing.T) {
	es, _ := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{"http://localhost:9999"},
	})
	processor := heartbeat.NewHeartbeatProcessor(es, logger.New(io.Discard, "test"))

	hb := gen.Heartbeat{
		ServiceId: "test-service",
		Timestamp: time.Now().UTC(),
	}
	body, _ := xml.Marshal(hb)

	err := processor(body)
	assert.Error(t, err, "should fail when ES unavailable")
	assert.Contains(t, err.Error(), "index")
}
