package heartbeat

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"integration-project-ehb/controlroom/pkg/gen"
	"integration-project-ehb/controlroom/pkg/logger"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"
)

func indexHeartbeat(es *elasticsearch.Client, ctx context.Context, hb *gen.Heartbeat) error {

	sId := strings.ToLower(hb.ServiceId)
	logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("heartbeat: indexing heartbeat for %s", sId)))

	if es == nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, "heartbeat: Elasticsearch client is nil"))
		return fmt.Errorf("elasticsearch client is nil")
	}

	doc := gen.HeartbeatDoc{
		ServiceId: sId,
		Timestamp: hb.Timestamp,
		Indexed:   time.Now(),
	}

	jsonData, err := doc.MarshalJSON()
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("heartbeat: failed to marshal heartbeat for %s: %v", sId, err)))
		return err
	}

	req := esapi.IndexRequest{
		Index:      "heartbeats",
		DocumentID: fmt.Sprintf("%s-%d", sId, hb.Timestamp.Unix()),
		Body:       bytes.NewReader(jsonData),
		Refresh:    "false",
	}

	go func() {
		res, err := req.Do(ctx, es)

		if err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("heartbeat: failed to index heartbeat for %s: %v", sId, err)))
			// return err
		}

		defer func(Body io.ReadCloser) {
			if err := Body.Close(); err != nil {
				logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, fmt.Sprintf("heartbeat: failed to close response body after indexing %s: %v", sId, err)))
			}
		}(res.Body)

		if res.IsError() {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("heartbeat: Elasticsearch error indexing heartbeat for %s: %s", sId, res.String())))
			// return fmt.Errorf("elasticsearch error: %s", res.String())
		}

	}()

	return nil
}
