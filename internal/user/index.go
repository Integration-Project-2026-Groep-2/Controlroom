package userobject

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"integration-project-ehb/controlroom/pkg/gen"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"
)

// indexUser marshals a UserConfirmed to JSON and indexes it in Elasticsearch.
func indexUser(es *elasticsearch.Client, ctx context.Context, uo *gen.UserConfirmed) error {
	doc := map[string]any{
		"id":      uo.Id,
		"role":    uo.Role,
		"indexed": time.Now(),
	}

	jsonData, err := json.Marshal(doc)
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

		}
	}(res.Body)

	if res.IsError() {
		return fmt.Errorf("elasticsearch: %s", res.String())
	}

	return nil
}
