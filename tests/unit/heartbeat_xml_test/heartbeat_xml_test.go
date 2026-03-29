// Package heartbeat_xml_test test of de XML structuur van Heartbeat correct werkt.
//
// Test 1 (MarshalUnmarshal): zet een Heartbeat struct om naar XML en terug naar een struct.
// Controleert of ServiceId en Timestamp hetzelfde blijven na het omzetten.
//
// Test 2 (XMLTagNames): controleert of de XML de juiste tagnamen heeft (<heartbeat>, <serviceId>).
// Dit is belangrijk omdat andere services berichten sturen met deze exacte tagnamen.
// Als de tags veranderen, begrijpen services elkaar niet meer.
package main

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"integration-project-ehb/controlroom/pkg/xml/gen"
)

func TestHeartbeat_MarshalUnmarshal(t *testing.T) {
	original := gen.Heartbeat{
		ServiceId: "test-service",
		Timestamp: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	data, err := xml.Marshal(original)
	assert.NoError(t, err)

	var result gen.Heartbeat
	err = xml.Unmarshal(data, &result)
	assert.NoError(t, err)

	assert.Equal(t, original.ServiceId, result.ServiceId)
	assert.Equal(t, original.Timestamp.UTC(), result.Timestamp.UTC())
}

func TestHeartbeat_XMLTagNames(t *testing.T) {
	hb := gen.Heartbeat{ServiceId: "svc", Timestamp: time.Now().UTC()}

	data, err := xml.Marshal(hb)
	assert.NoError(t, err)

	xmlStr := string(data)
	assert.True(t, strings.Contains(xmlStr, "<heartbeat>"), "root tag moet <heartbeat> zijn")
	assert.True(t, strings.Contains(xmlStr, "<serviceId>"), "veld tag moet <serviceId> zijn")
}
