package user

import (
	"bytes"
	"context"
	"fmt"
	"integration-project-ehb/controlroom/pkg/gen"
	"io"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"
	"github.com/mailru/easyjson"
)

// indexUser marshals a UserConfirmed to JSON and indexes it in Elasticsearch.
func indexUser(es *elasticsearch.Client, ctx context.Context, uo *gen.UserConfirmed) error {

	doc := gen.UserDoc{
		Id:      uo.Id,
		Role:    uo.Role,
		Indexed: time.Now(),
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
			// TODO(nasr): what happens here then? this is a fix suggested by the static analyzers
			// but when the closing does fail? how do you handle it?
		}
	}(res.Body)

	if res.IsError() {
		return fmt.Errorf("elasticsearch: %s", res.String())
	}

	return nil
}
