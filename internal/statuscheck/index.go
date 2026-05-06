package statuscheck

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"integration-project-ehb/controlroom/pkg/gen"
	"integration-project-ehb/controlroom/pkg/logger"
	"io"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"
)

func indexStatusCheck(es *elasticsearch.Client, ctx context.Context, sct *gen.StatusCheck) error {

	logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("indexing statuscheck for %s", sct.ServiceId)))

	doc := gen.StatusCheckDoc{
		ServiceId: sct.ServiceId,
		Timestamp: sct.Timestamp,
		Uptime:    sct.Uptime,
		Memory:    sct.Memory,
		Disk:      sct.Disk,
	}

	jsonData, err := json.Marshal(doc)
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("failed to marshal statuscheck for %s: %v", sct.ServiceId, err)))
		return err
	}

	req := esapi.IndexRequest{
		Index:      "statuscheck",
		DocumentID: fmt.Sprintf("%s-%d", sct.ServiceId, sct.Timestamp.Unix()),
		Body:       bytes.NewReader(jsonData),
		Refresh:    "true",
	}

	res, err := req.Do(ctx, es)
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("failed to index statuscheck for %s: %v", sct.ServiceId, err)))
		return err
	}
	defer func(Body io.ReadCloser) {
		if err := Body.Close(); err != nil {
			logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, fmt.Sprintf("failed to close body after indexing statuscheck %s: %v", sct.ServiceId, err)))
		}
	}(res.Body)

	if res.IsError() {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("elasticsearch error indexing statuscheck for %s: %s", sct.ServiceId, res.String())))
		return fmt.Errorf("elasticsearch error: %s", res.String())
	}

	// logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, fmt.Sprintf("indexed statuscheck for %s", sct.ServiceId)))
	return nil
}
