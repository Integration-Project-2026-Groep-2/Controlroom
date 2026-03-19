package elastic

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"
)

// Helper to push to Elasticsearch
func SendToElastic(es *elasticsearch.Client, index string, data interface{}) {
	jsonValue, _ := json.Marshal(data)
	req := esapi.IndexRequest{
		Index:   index,
		Body:    bytes.NewReader(jsonValue),
		Refresh: "true",
	}
	res, _ := req.Do(context.Background(), es)
	if res != nil {
		res.Body.Close()
	}
}
