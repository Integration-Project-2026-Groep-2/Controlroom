package statuscheck

import (
	"context"
	"encoding/xml"
	"fmt"
	"log"
	"time"

	"integration-project-ehb/controlroom/pkg/gen"

	"github.com/elastic/go-elasticsearch/v9"
)

func NewStatusCheckProcessor(es *elasticsearch.Client) func([]byte) error {
	return func(body []byte) error {
		var sct gen.StatusCheck
		if err := xml.Unmarshal(body, &sct); err != nil {
			return fmt.Errorf("unmarshal: %w", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := indexStatusCheck(es, ctx, &sct); err != nil {
			return fmt.Errorf("index: %w", err)
		}

		log.Printf("Indexed status check: %s", sct.ServiceId)
		return nil
	}
}
