package user

import (
	"context"
	"encoding/xml"
	"fmt"
	"time"

	"github.com/elastic/go-elasticsearch/v9"

	"integration-project-ehb/controlroom/pkg/gen"
	"integration-project-ehb/controlroom/pkg/logger"
)

// NOTE(nasr): dead letter queue handling is now centralized in cr_rabbitmq.Consumer.
// Removes boilerplate and simplifies code generation in the future.
// NewUserProcessor unmarshals, validates, and indexes a user message.
// Returns error to trigger DLQ routing via cr_rabbitmq consumer.

// NOTE(nasr): because i keep forgetting why I used this ugly syntax im going to be writing it here
// we return a processor that with the signature of func([]byte) because that fits properly in the
// ConsumerConfig struct. we made that a struct so we could avoid using a global one
// not sure if this is the correct approach. probably not
func NewUserProcessor(es *elasticsearch.Client) func([]byte) error {
	return func(body []byte) error {
		var uc gen.UserConfirmed
		if err := xml.Unmarshal(body, &uc); err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("Unmarshal error when trying to unmarshal user xml: %v", err.Error())))
			return fmt.Errorf("unmarshal error when trying to unmarshal user xml: %v", err.Error())
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := indexUser(es, ctx, &uc); err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("Failed to index user object: %s", err.Error())))
			return fmt.Errorf("Failed to index user object: %s", err.Error())
		}

		logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, fmt.Sprintf("Indexed user object: %s", uc.Id)))
		return nil
	}
}
