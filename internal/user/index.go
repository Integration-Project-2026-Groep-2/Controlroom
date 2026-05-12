package user

import (
	"bytes"
	"context"
	"fmt"
	"integration-project-ehb/controlroom/pkg/gen"
	"integration-project-ehb/controlroom/pkg/logger"
	"io"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"
	"github.com/mailru/easyjson"
)

// indexUser marshals a UserConfirmed to JSON and indexes it in Elasticsearch.
func indexUser(es *elasticsearch.Client, ctx context.Context, uo *gen.UserConfirmed) error {

	logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, "indexing user"))

	doc := gen.UserDoc{
		Id:        uo.Id,
		Role:      uo.Role,
		CompanyId: uo.CompanyId,
		Indexed:   time.Now(),
	}

	jsonData, err := easyjson.Marshal(doc)

	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	req := esapi.IndexRequest{
		Index:      "users",
		DocumentID: fmt.Sprintf("%s-%s", uo.Id, uo.ConfirmedAt),
		Body:       bytes.NewReader(jsonData),
		Refresh:    "true",
	}

	res, err := req.Do(ctx, es)
	if err != nil {
		return fmt.Errorf("index: %w", err)
	}

	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, fmt.Sprintf("failed to close reader when indexing user: %v", err)))
		}
	}(res.Body)

	if res.IsError() {
		logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, fmt.Sprintf("response contains an error when indexing user: %v", res.String())))
		return fmt.Errorf("elasticsearch: %s", res.String())
	}

	return nil
}
