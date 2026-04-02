package userobject

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"
	"integration-project-ehb/controlroom/pkg/gen"
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
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("elasticsearch: %s", res.String())
	}

	return nil
}
