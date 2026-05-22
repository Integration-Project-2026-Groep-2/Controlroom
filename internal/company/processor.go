package company

import (
	"context"
	"encoding/xml"
	"fmt"
	"time"

	"integration-project-ehb/controlroom/pkg/gen"

	"integration-project-ehb/controlroom/pkg/logger"

	"github.com/elastic/go-elasticsearch/v9"
)

func ProcessCompany(es *elasticsearch.Client, body []byte) error {
	var company gen.CompanyConfirmed
	if err := xml.Unmarshal(body, &company); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("company: failed to unmarshal XML: %v", err)))
		return fmt.Errorf("company: failed to unmarshal XML: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := indexCompany(es, ctx, &company); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("company: failed to index company %s: %v", company.Id, err)))
		return fmt.Errorf("company: failed to index company: %w", err)
	}
	logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, fmt.Sprintf("company: indexed company %s", company.Id)))
	return nil
}
