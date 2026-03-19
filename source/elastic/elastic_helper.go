package elastic



// Helper to push to Elasticsearch
func sendToElastic(es *elasticsearch.Client, index string, data interface{}) {
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
