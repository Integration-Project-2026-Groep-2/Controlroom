package unit_tests

import (
	"testing"

	"integration-project-ehb/controlroom/cmd/mcp"

	"github.com/stretchr/testify/assert"
)

func TestBuildFetchLogsQuery_BoolFilterShape(t *testing.T) {
	q := mcp.BuildFetchLogsQuery("kassa", "2026-05-10T14:18:17Z", "2026-05-10T14:24:17Z")

	boolPart, ok := q["bool"].(map[string]any)
	assert.True(t, ok, "top-level must be 'bool'")

	filter, ok := boolPart["filter"].([]any)
	assert.True(t, ok, "bool.filter must be array")
	assert.Len(t, filter, 3, "expected term + range + terms clauses")
}

func TestBuildFetchLogsQuery_ServiceTerm(t *testing.T) {
	q := mcp.BuildFetchLogsQuery("kassa", "x", "y")
	filter := q["bool"].(map[string]any)["filter"].([]any)
	term := filter[0].(map[string]any)["term"].(map[string]any)
	assert.Equal(t, "kassa", term["service.keyword"])
}

func TestBuildFetchLogsQuery_TimestampRange(t *testing.T) {
	q := mcp.BuildFetchLogsQuery("kassa", "2026-05-10T14:18:17Z", "2026-05-10T14:24:17Z")
	filter := q["bool"].(map[string]any)["filter"].([]any)
	rng := filter[1].(map[string]any)["range"].(map[string]any)["timestamp"].(map[string]any)
	assert.Equal(t, "2026-05-10T14:18:17Z", rng["gte"])
	assert.Equal(t, "2026-05-10T14:24:17Z", rng["lte"])
}

func TestBuildFetchLogsQuery_LevelTermsErrorAndWarn(t *testing.T) {
	q := mcp.BuildFetchLogsQuery("kassa", "x", "y")
	filter := q["bool"].(map[string]any)["filter"].([]any)
	terms := filter[2].(map[string]any)["terms"].(map[string]any)["level.keyword"].([]string)
	assert.ElementsMatch(t, []string{"ERROR", "WARN"}, terms)
}
