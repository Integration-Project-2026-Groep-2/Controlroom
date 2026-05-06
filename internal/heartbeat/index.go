package heartbeat

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"integration-project-ehb/controlroom/pkg/gen"
	"integration-project-ehb/controlroom/pkg/logger"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"
)

func indexHeartbeat(es *elasticsearch.Client, ctx context.Context, hb *gen.Heartbeat) error {
	logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("indexing heartbeat for %s", hb.ServiceId)))

	if es == nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, "elasticsearch client is nil"))
		return fmt.Errorf("elasticsearch client is nil")
	}

	doc := gen.HeartbeatDoc{
		ServiceId: hb.ServiceId,
		Timestamp: hb.Timestamp,
		Indexed:   time.Now(),
	}

	jsonData, err := doc.MarshalJSON()
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("failed to marshal heartbeat for %s: %v", hb.ServiceId, err)))
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
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("failed to index heartbeat for %s: %v", hb.ServiceId, err)))
		return err
	}
	defer func(Body io.ReadCloser) {
		if err := Body.Close(); err != nil {
			logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, fmt.Sprintf("failed to close body after indexing heartbeat %s: %v", hb.ServiceId, err)))
		}
	}(res.Body)

	if res.IsError() {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("elasticsearch error indexing heartbeat for %s: %s", hb.ServiceId, res.String())))
		return fmt.Errorf("elasticsearch error: %s", res.String())
	}

	// logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, fmt.Sprintf("indexed heartbeat for %s", hb.ServiceId)))
	return nil
}
