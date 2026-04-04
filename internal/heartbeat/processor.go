package heartbeat

import (
	"context"
	"encoding/xml"
	"fmt"
	"time"

	"integration-project-ehb/controlroom/pkg/gen"
	"integration-project-ehb/controlroom/pkg/logger"

	"github.com/elastic/go-elasticsearch/v9"
)

// NOTE(nasr): returning a function independenant of es client to simplify mocking
func NewHeartbeatProcessor(es *elasticsearch.Client, log *logger.Logger) func([]byte) error {
	return func(body []byte) error {
		var hb gen.Heartbeat
		if err := xml.Unmarshal(body, &hb); err != nil {
			return fmt.Errorf("unmarshal: %w", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := indexHeartbeat(es, ctx, &hb); err != nil {
			return fmt.Errorf("index: %w", err)
		}

		log.Info("indexed heartbeat", logger.String("service_id", hb.ServiceId))
		return nil
	}
}
