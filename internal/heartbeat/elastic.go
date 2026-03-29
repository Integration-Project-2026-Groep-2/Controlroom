package heartbeat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"integration-project-ehb/controlroom/pkg/xml_gen"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"
)

func indexHeartbeat(es *elasticsearch.Client, ctx context.Context, hb *xml_gen.Heartbeat) error {
	// TODO(nasr): find a way to replace this witth a generated reference.
	// we dont want to make an document indexer per new recceiver
	doc := map[string]interface{}{
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
