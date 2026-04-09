package company

import (
	"context"
	"encoding/xml"
	"fmt"
	"log"
	"time"

	"integration-project-ehb/controlroom/pkg/gen"

	"github.com/elastic/go-elasticsearch/v9"
)

// NewCompanyProcessor ProcessCompany unmarshals, validates, and indexes a company message.
// Returns error to trigger DLQ routing via cr_rabbitmq consumer.
func NewCompanyProcessor(es *elasticsearch.Client) func([]byte) error {
	return func(body []byte) error {

		var company gen.CompanyConfirmed
		if err := xml.Unmarshal(body, &company); err != nil {
			return fmt.Errorf("unmarshal: %w", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := indexCompany(es, ctx, &company); err != nil {
			return fmt.Errorf("index: %w", err)
		}

		// TODO(nasr): perf optimization and replace with internal logger
		log.Printf("Indexed company object: %s", company.Id)
		return nil
	}
}

// NOTE(nasr): dead letter queue handling is now centralized in cr_rabbitmq.Consumer.
// Removes boilerplate and simplifies code generation in the future.
