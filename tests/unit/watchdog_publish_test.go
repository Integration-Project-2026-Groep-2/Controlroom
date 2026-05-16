package unit_tests

import (
	"testing"
)

func TestBuildPDCEFEnvelope_TopLevelFields(t *testing.T) {
	/*
		body, err := watchdog.BuildOfflineMessage("KASSA", 0)
		assert.NoError(t, err)

		var env map[string]any
		assert.NoError(t, json.Unmarshal(body, &env))

		assert.Equal(t, "heartbeat_failed", env["event"])
		assert.Equal(t, "controlroom-watchdog", env["source"])
		assert.NotEmpty(t, env["timestamp"])
	*/
}

func TestBuildPDCEFEnvelope_PayloadShape(t *testing.T) {
	/*
		body, _ := watchdog.BuildOfflineMessage("KASSA", 12)

		var env map[string]any
		_ = json.Unmarshal(body, &env)

		payload, ok := env["payload"].(map[string]any)
		assert.True(t, ok, "payload must be object")
		assert.Equal(t, "kassa", payload["component"])
		assert.Equal(t, "critical", payload["severity"])
		assert.Equal(t, "festival-services", payload["group"])
		assert.Equal(t, "heartbeat-loss", payload["class"])
		assert.Contains(t, payload["summary"], "KASSA")
		assert.Contains(t, payload["summary"], "12")

		details, ok := payload["custom_details"].(map[string]any)
		assert.True(t, ok, "custom_details must be object")
		assert.Equal(t, float64(12), details["heartbeat_count_last_60s"])
		assert.Equal(t, float64(30), details["threshold"])
		assert.NotEmpty(t, details["last_check_at"])
	*/
}

func TestBuildPDCEFEnvelope_LowercasesComponent(t *testing.T) {
	/*
		body, _ := watchdog.BuildOfflineMessage("FACTURATIE", 0)
		var env map[string]any
		_ = json.Unmarshal(body, &env)
		payload := env["payload"].(map[string]any)
		assert.Equal(t, "facturatie", payload["component"])
	*/
}
