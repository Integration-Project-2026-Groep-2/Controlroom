package processor_test

import (
	"encoding/xml"
	"testing"
	"time"

	"integration-project-ehb/controlroom/internal/heartbeat"
	"integration-project-ehb/controlroom/pkg/gen"

	"github.com/stretchr/testify/assert"
)

// TestProcessHeartbeat_InvalidXML: malformed XML should fail during unmarshal
func TestProcessHeartbeat_InvalidXML(t *testing.T) {
	err := heartbeat.ProcessHeartbeat(unreachableES(t), []byte("invalid xml"))
	assert.Error(t, err, "should fail on malformed XML")
	assert.Contains(t, err.Error(), "unmarshal")
}

// TestProcessHeartbeat_ValidXML_ESUnavailable: valid XML but no ES -> index error
func TestProcessHeartbeat_ValidXML_ESUnavailable(t *testing.T) {
	hb := gen.Heartbeat{ServiceId: "test-service", Timestamp: time.Now().UTC()}
	body, _ := xml.Marshal(hb)
	err := heartbeat.ProcessHeartbeat(unreachableES(t), body)
	assert.Error(t, err, "should fail when ES unavailable")
	assert.Contains(t, err.Error(), "index")
}
