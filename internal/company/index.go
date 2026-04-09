package company

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"integration-project-ehb/controlroom/pkg/gen"
	"io"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"
)

// indexCompany indexes consumed companies to elastic
func indexCompany(es *elasticsearch.Client, ctx context.Context, comp *gen.CompanyConfirmed) error {
	doc := map[string]any{
		"Id": comp.Id,
		/* NOTE(nasr): dont store these? gdpr?
		* "Email":     comp.VatNumber,
		* "FirstName": comp.Name,
		* "LastName":  comp.Email,
		* "Phone":     comp.IsActive, */
		"Role":    comp.ConfirmedAt,
		"indexed": time.Now(),
	}

	jsonData, err := json.Marshal(doc)
	if err != nil {
		return err
	}

	req := esapi.IndexRequest{
		Index:      "companies",
		DocumentID: fmt.Sprintf("%s-%v", comp.Id, comp.ConfirmedAt),
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

	// TODO(nasr): check if this is a redundant check...
	if res.IsError() {
		return fmt.Errorf("elasticsearch error: %s", res.String())
	}

	return nil
}
