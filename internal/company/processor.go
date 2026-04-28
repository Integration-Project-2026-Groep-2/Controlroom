package company

import (
	"context"
	"encoding/xml"
	"fmt"
	"time"

	"integration-project-ehb/controlroom/pkg/gen"

	"github.com/elastic/go-elasticsearch/v9"
	"integration-project-ehb/controlroom/pkg/logger"
)

func NewCompanyProcessor(es *elasticsearch.Client) func([]byte) error {
	return func(body []byte) error {

		var company gen.CompanyConfirmed
		if err := xml.Unmarshal(body, &company); err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("Unmarshal error when trying to unmarshal company xml: %v", err.Error())))
			return fmt.Errorf("unmarshal error when trying to unmarshal company xml: %v", err.Error())

		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := indexCompany(es, ctx, &company); err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("Failed to index user object: %s", err.Error())))
			return fmt.Errorf("Failed to index company object: %s", err.Error())
		}

		logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, fmt.Sprintf("Indexed company object: %s", company.Id)))
		return nil
	}
}
