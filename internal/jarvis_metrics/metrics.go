package jarvis_metrics

import (
	"context"
	"fmt"
	config "integration-project-ehb/controlroom/internal/cr_config"
	"net/http"
	"time"
)

type McpDataLabel struct {
	Route    string
	Quantile float64
}

type McpData struct {
	Timestamp time.Time `json:"timestamp" validate:"required"`
	// Metric    time.Time `json:"timestamp" validate:"required"`
	Value float64 `json:"integer" validate:"required"`
}

func RetrieveMetrics(context context.Context, client *http.Client) (McpData, error) {

	req, err := http.NewRequestWithContext(context, http.MethodGet, config.McpMasterUrl, nil)
	if err != nil {
		return McpData{}, nil
	}

	// TODO(nasr): do we need a header token for the token

	resp, err := client.Do(req)
	if err != nil {
		return McpData{}, nil
	}

	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return McpData{}, fmt.Errorf("bad error response, %v", resp.StatusCode)
	}

	return McpData{}, nil
}
