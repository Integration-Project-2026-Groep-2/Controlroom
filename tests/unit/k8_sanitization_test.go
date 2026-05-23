package unit_tests

import (
	m "integration-project-ehb/controlroom/internal/k8retriever"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeStringMapReplacesDotsAndSlashes(t *testing.T) {
	input := map[string]string{
		"app.kubernetes.io/component": "rabbitmq",
		"simple":                      "value",
	}

	got := m.Sanitize(input)

	require.Equal(t, map[string]string{
		"app_kubernetes_io_component": "rabbitmq",
		"simple":                      "value",
	}, got)
	require.Equal(t, map[string]string{
		"app.kubernetes.io/component": "rabbitmq",
		"simple":                      "value",
	}, input)
}

func TestSanitizeStringMapNilForEmptyInput(t *testing.T) {
	require.Nil(t, m.Sanitize(nil))
	require.Nil(t, m.Sanitize(map[string]string{}))
}
