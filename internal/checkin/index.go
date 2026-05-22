package checkin

import (
	"bytes"
	"context"
	"fmt"

	"integration-project-ehb/controlroom/pkg/gen"
	"integration-project-ehb/controlroom/pkg/logger"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"
	"github.com/mailru/easyjson"
)

func indexCheckIn(es *elasticsearch.Client, ctx context.Context, ci *gen.CheckIn) error {
	logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("checkin: indexing checkin for %s", ci.Id)))

	if es == nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, "checkin: Elasticsearch client is nil"))
		return fmt.Errorf("elasticsearch client is nil")
	}

	doc := gen.CheckInDoc{
		Id:        ci.Id,
		Timestamp: ci.Timestamp,
	}

	jsonData, err := easyjson.Marshal(doc)

	if err != nil {
		logger.Log(logger.NewMessage(
			logger.ERROR,
			logger.CONTROLROOM,
			fmt.Sprintf("checkin: failed to marshal checkin for %s: %v", ci.Id, err),
		))
		return err
	}

	req := esapi.IndexRequest{
		Index:      "checkins",
		DocumentID: fmt.Sprintf("%s-%d", ci.Id, ci.Timestamp.Unix()),
		Body:       bytes.NewReader(jsonData),
		Refresh:    "wait_for",
	}

	res, err := req.Do(ctx, es)
	if err != nil {
		logger.Log(logger.NewMessage(
			logger.ERROR,
			logger.CONTROLROOM,
			fmt.Sprintf("checkin: failed to index checkin for %s: %v", ci.Id, err),
		))
		return err
	}
	defer func() {
		if err := res.Body.Close(); err != nil {
			logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, fmt.Sprintf("checkin: failed to close response body after indexing %s: %v", ci.Id, err)))
		}
	}()

	if res.IsError() {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("checkin: Elasticsearch error indexing checkin for %s: %s", ci.Id, res.String())))
		return fmt.Errorf("elasticsearch error: %s", res.String())
	}

	return nil
}
