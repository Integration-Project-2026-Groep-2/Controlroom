package statuscheck

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"integration-project-ehb/controlroom/pkg/gen"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"
)

func indexStatusCheck(es *elasticsearch.Client, ctx context.Context, sct *gen.StatusCheckType) error {

	doc := map[string]any{
		"service_id":  sct.ServiceId,
		"timestamp":   sct.Timestamp,
		"status":      sct.Status,
		"uptime":      sct.Uptime,
		"system_load": sct.SystemLoad,
		"indexed":     time.Now(),
	}

	jsonData, err := json.Marshal(doc)
	if err != nil {
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
		return err
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("elasticsearch error: %s", res.String())
	}

	return nil
}
