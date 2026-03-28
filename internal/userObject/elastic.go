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

func indexUser(es *elasticsearch.Client, ctx context.Context, uo *gen.UserConfirmed) error {
	doc := map[string]interface{}{
		"Id":          uo.Id,
		"Email":       uo.Email,
		"FirstName":   uo.FirstName,
		"LastName":    uo.LastName,
		"Phone":       uo.Phone,
		"Role":        uo.Role,
		"CompanyId":   uo.CompanyId,
		"BadgeCode":   uo.BadgeCode,
		"IsActive":    uo.IsActive,
		"GdprConsent": uo.GdprConsent,
		"ConfirmedAt": uo.ConfirmedAt,
		"indexed":     time.Now(),
	}

	jsonData, err := json.Marshal(doc)
	if err != nil {
		return err
	}

	req := esapi.IndexRequest{
		Index:      "users",
		DocumentID: fmt.Sprintf("%s-%d", uo.Id, uo.ConfirmedAt),
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
