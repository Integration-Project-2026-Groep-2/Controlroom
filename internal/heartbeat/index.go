package heartbeat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"integration-project-ehb/controlroom/pkg/gen"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"
)

func indexHeartbeat(es *elasticsearch.Client, ctx context.Context, hb *gen.Heartbeat) error {

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
