package statuscheck

import (
	"context"
	"encoding/xml"
	"fmt"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	"integration-project-ehb/controlroom/pkg/gen"
	"integration-project-ehb/controlroom/pkg/logger"
)

func NewStatusCheckProcessor(es *elasticsearch.Client, log *logger.Logger) func([]byte) error {
	return func(body []byte) error {
		var sct gen.StatusCheckType
		if err := xml.Unmarshal(body, &sct); err != nil {
			return fmt.Errorf("unmarshal: %w", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := indexStatusCheck(es, ctx, &sct); err != nil {
			return fmt.Errorf("index: %w", err)
		}

		log.Info("indexed status check", logger.String("service_id", sct.ServiceId))
		return nil
	}
}
