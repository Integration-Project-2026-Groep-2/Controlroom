package heartbeat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	// =============================================================================
	"github.com/elastic/go-elasticsearch/v9"
	// esapi: api endpoints for elastic
	"github.com/elastic/go-elasticsearch/v9/esapi"
	// =============================================================================

	// =============================================================================
	"integration-project-ehb/controlroom/pkg/xml/gen"
	"integration-project-ehb/controlroom/pkg/logger/logger"
	// =============================================================================
)

func indexHeartbeat(es *elasticsearch.Client, ctx context.Context, hb *gen.Heartbeat) error {
	// NOTE(nasr): in elastic every record is called a document
	doc := map[string]any{
		"serviceId": hb.ServiceId,
		"timestamp": hb.Timestamp,
		"indexed":   time.Now(),
	}

	jsonData, err := json.Marshal(doc)
	if err != nil {
		return err
	}

	req := esapi.IndexRequest{
		Index:      "heartbeats",
		DocumentID: fmt.Sprintf("%s-%d", hb.ServiceId, hb.Timestamp.Unix()),
		Body:       bytes.NewReader(jsonData),
		Refresh:    "true",
	}

	res, err := req.Do(ctx, es)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("elasticsearch error: %s", res.String())
	}

	return nil
}

func indexInternalLogs(es *elasticsearch.Client, ctx context.Context, logMsg *internal_logger.LogMessage) error {
	// Build the document to index
	doc := map[string]any{
		"message":   logMsg.Message,
		"error":     logMsg.Error,
		"service":   logMsg.Service,
		"severity":  logMsg.Severity,
		"timestamp": logMsg.Timestamp,
		"indexed":   time.Now(),
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("failed to marshal log message: %w", err)
	}

	// Create an IndexRequest
	req := esapi.IndexRequest{
		Index:      "internal-logs", // You can choose your index name
		DocumentID: fmt.Sprintf("%s-%d", logMsg.Service, logMsg.Timestamp.UnixNano()),
		Body:       bytes.NewReader(jsonData),
		Refresh:    "true", // optional: refresh immediately
	}

	// Execute the request
	res, err := req.Do(ctx, es)
	if err != nil {
		return fmt.Errorf("failed to send log to Elasticsearch: %w", err)
	}
	defer res.Body.Close()

	// Check for errors in the response
	if res.IsError() {
		return fmt.Errorf("elasticsearch error: %s", res.String())
	}

	return nil
}
