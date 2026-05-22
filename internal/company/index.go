package company

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"integration-project-ehb/controlroom/pkg/gen"
	"integration-project-ehb/controlroom/pkg/logger"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"
	"github.com/mailru/easyjson"
)

func indexCompany(es *elasticsearch.Client, ctx context.Context, comp *gen.CompanyConfirmed) error {
	logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("company: indexing company %s", comp.Id)))

	doc := gen.CompanyDoc{
		Id:          comp.Id,
		Name:        comp.Name,
		ConfirmedAt: comp.ConfirmedAt,
		Indexed:     time.Now(),
	}

	jsonData, err := easyjson.Marshal(doc)
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("company: failed to marshal company %s: %v", comp.Id, err)))
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
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("company: failed to index company %s: %v", comp.Id, err)))
		return fmt.Errorf("index: %w", err)
	}
	defer func(Body io.ReadCloser) {
		if err := Body.Close(); err != nil {
			logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, fmt.Sprintf("company: failed to close response body after indexing %s: %v", comp.Id, err)))
		}
	}(res.Body)

	if res.IsError() {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("company: Elasticsearch error indexing company %s: %s", comp.Id, res.String())))
		return fmt.Errorf("elasticsearch error: %s", res.String())
	}

	logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, fmt.Sprintf("company: indexed company %s", comp.Id)))

	// ==============================================================================
	// NIEUW: AUTOMATISATIE VAN DE ENRICH POLICY
	// ==============================================================================
	enrichReq := esapi.EnrichExecutePolicyRequest{
		Name: "company_lookup",
	}

	enrichRes, enrichErr := enrichReq.Do(ctx, es)
	if enrichErr != nil {
		// We gebruiken hier WARN in plaats van ERROR, omdat het bedrijf wel al succesvol is opgeslagen.
		// Een falende policy update mag de rest van de applicatie niet laten crashen.
		logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, fmt.Sprintf("company: failed to trigger enrich policy after indexing %s: %v", comp.Id, enrichErr)))
	} else {
		defer enrichRes.Body.Close()
		if enrichRes.IsError() {
			logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, fmt.Sprintf("company: Elasticsearch error executing enrich policy: %s", enrichRes.String())))
		} else {
			logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, fmt.Sprintf("company: refreshed company_lookup policy for new company %s", comp.Id)))
		}
	}
	// ==============================================================================

	return nil
}
