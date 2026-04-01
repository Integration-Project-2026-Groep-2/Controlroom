package userobject

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"integration-project-ehb/controlroom/pkg/xml/gen"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"
)

func indexCompany(es *elasticsearch.Client, ctx context.Context, comp *gen.CompanyConfirmed) error {
	doc := map[string]any{
		"Id":        comp.Id,
		"Email":     comp.VatNumber,
		"FirstName": comp.Name,
		"LastName":  comp.Email,
		"Phone":     comp.IsActive,
		"Role":      comp.ConfirmedAt,
		"indexed":   time.Now(),
	}

	jsonData, err := json.Marshal(doc)
	if err != nil {
		return err
	}

	req := esapi.IndexRequest{
		Index:      "users",
		DocumentID: fmt.Sprintf("%s-%v", comp.Id, comp.ConfirmedAt),
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
