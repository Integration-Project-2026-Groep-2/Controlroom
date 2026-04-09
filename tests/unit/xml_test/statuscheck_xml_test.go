package xml_test

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"integration-project-ehb/controlroom/pkg/gen"
)

func TestStatusCheck_MarshalUnmarshal(t *testing.T) {
	original := gen.StatusCheck{
		ServiceId: "crm-service",
		Timestamp: time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC),
		Uptime:    3600,
	}

	data, err := xml.Marshal(original)
	assert.NoError(t, err)

	var result gen.StatusCheck
	err = xml.Unmarshal(data, &result)
	assert.NoError(t, err)

	assert.Equal(t, original.ServiceId, result.ServiceId)
	assert.Equal(t, original.Uptime, result.Uptime)
	assert.Equal(t, original.Timestamp.UTC(), result.Timestamp.UTC())
}

func TestStatusCheck_XMLTagNames(t *testing.T) {
	sc := gen.StatusCheck{
		ServiceId: "kassa-service",
		Timestamp: time.Now().UTC(),
		Uptime:    120,
	}

	data, err := xml.Marshal(sc)
	assert.NoError(t, err)

	xmlStr := string(data)
	assert.True(t, strings.Contains(xmlStr, "<StatusCheck>"), "expected root tag <StatusCheck>")
	assert.True(t, strings.Contains(xmlStr, "<serviceId>"), "expected tag <serviceId>")
	assert.True(t, strings.Contains(xmlStr, "<uptime>"), "expected tag <uptime>")
	assert.True(t, strings.Contains(xmlStr, "<timestamp>"), "expected tag <timestamp>")
}

func TestStatusCheck_UptimeZero(t *testing.T) {
	// uptime=0 is valid (service just started), should round-trip cleanly
	sc := gen.StatusCheck{
		ServiceId: "facturatie-service",
		Timestamp: time.Now().UTC(),
		Uptime:    0,
	}

	data, err := xml.Marshal(sc)
	assert.NoError(t, err)

	var result gen.StatusCheck
	err = xml.Unmarshal(data, &result)
	assert.NoError(t, err)
	assert.Equal(t, uint(0), result.Uptime)
}

func TestStatusCheck_EmptyServiceId(t *testing.T) {
	// empty serviceId should still marshal/unmarshal without error,
	// validation is a separate concern
	sc := gen.StatusCheck{
		Timestamp: time.Now().UTC(),
		Uptime:    999,
	}

	data, err := xml.Marshal(sc)
	assert.NoError(t, err)

	var result gen.StatusCheck
	err = xml.Unmarshal(data, &result)
	assert.NoError(t, err)
	assert.Equal(t, "", result.ServiceId)
}

func TestStatusCheck_InvalidXML(t *testing.T) {
	var sc gen.StatusCheck
	err := xml.Unmarshal([]byte("not xml at all"), &sc)
	assert.Error(t, err)
}

func TestSystemLoad_MarshalUnmarshal(t *testing.T) {
	original := gen.SystemLoad{
		Cpu:    0.72,
		Memory: 0.55,
		Disk:   0.30,
	}

	data, err := xml.Marshal(original)
	assert.NoError(t, err)

	var result gen.SystemLoad
	err = xml.Unmarshal(data, &result)
	assert.NoError(t, err)

	assert.InDelta(t, original.Cpu, result.Cpu, 0.0001)
	assert.InDelta(t, original.Memory, result.Memory, 0.0001)
	assert.InDelta(t, original.Disk, result.Disk, 0.0001)
}
