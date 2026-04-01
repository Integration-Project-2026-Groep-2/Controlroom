package userobject

import (
	"context"
	"encoding/xml"
	"fmt"
	"log"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	"integration-project-ehb/controlroom/pkg/gen"
)

// ProcessUserObject unmarshals, validates, and indexes a user message.
// Returns error to trigger DLQ routing via cr_rabbitmq consumer.
func NewUserProcessor(es *elasticsearch.Client) func([]byte) error {
	return func(body []byte) error {
		var uc gen.UserConfirmed
		if err := xml.Unmarshal(body, &uc); err != nil {
			return fmt.Errorf("unmarshal: %w", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := indexUser(es, ctx, &uc); err != nil {
			return fmt.Errorf("index: %w", err)
		}

		log.Printf("Indexed user object: %s", uc.Id)
		return nil
	}
}

// NOTE(nasr): dead letter queue handling is now centralized in cr_rabbitmq.Consumer.
// Removes boilerplate and simplifies code generation in the future.
