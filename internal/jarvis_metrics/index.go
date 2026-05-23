package jarvis_metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"bytes"

	"integration-project-ehb/controlroom/pkg/logger"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"
)

// IndexMetrics indexes a slice of metric samples into Elasticsearch.
// Uses the same pattern as heartbeat indexing with gen.HeartbeatDoc.
func IndexMetrics(es *elasticsearch.Client, ctx context.Context, samples []Metrics) error {

	logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, "[DEBUG] indexing metrics"))

	if es == nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, "metrics: Elasticsearch client is nil"))
		return fmt.Errorf("elasticsearch client is nil")
	}

	if len(samples) == 0 {
		logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, "metrics: no samples to index"))
		return nil
	}

	indexed := 0
	failed := 0

	// TODO(nasr): replace this with the easy json implementation
	for _, sample := range samples {
		// Create a document struct similar to gen.HeartbeatDoc
		// Map MetricSample to a gen-compatible structure or use direct JSON
		doc := map[string]any{
			"@timestamp": sample.Timestamp,
			"metric":     sample.Metric,
			"value":      sample.Value,
			"labels":     sample.Labels,
			"indexed":    time.Now(),
		}

		jsonData, err := json.Marshal(doc)
		if err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("metrics: failed to marshal sample %s: %v", sample.Metric, err)))
			failed++
			continue
		}

		// DocumentID: metric-name + labels hash + timestamp for uniqueness
		docID := fmt.Sprintf("%s-%d", sample.Metric, sample.Timestamp.Unix())

		req := esapi.IndexRequest{
			Index:      "jarvis-data",
			DocumentID: docID,
			Body:       bytes.NewReader(jsonData),
			Refresh:    "true",
		}

		res, err := req.Do(ctx, es)
		if err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("metrics: failed to index sample %s: %v", sample.Metric, err)))
			failed++
			continue
		}

		defer func(Body io.ReadCloser) {
			if err := Body.Close(); err != nil {
				logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, fmt.Sprintf("metrics: failed to close response body for sample %s: %v", sample.Metric, err)))
			}
		}(res.Body)

		if res.IsError() {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("metrics: Elasticsearch error indexing sample %s: %s", sample.Metric, res.String())))
			failed++
			continue
		}

		indexed++
	}

	logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("metrics: indexed %d/%d samples (failed: %d)", indexed, len(samples), failed)))

	if failed > 0 {
		return fmt.Errorf("failed to index %d/%d metrics samples", failed, len(samples))
	}

	return nil
}
