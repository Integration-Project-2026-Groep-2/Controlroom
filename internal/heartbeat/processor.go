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

func ProcessHeartbeat(es *elasticsearch.Client, body []byte) error {

	var hb gen.Heartbeat

	if err := xml.Unmarshal(body, &hb); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("heartbeat: failed to unmarshal XML: %v", err)))
		return fmt.Errorf("heartbeat: failed to unmarshal XML: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := indexHeartbeat(es, ctx, &hb); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("heartbeat: failed to index heartbeat for %s: %v", hb.ServiceId, err)))
		return fmt.Errorf("heartbeat: failed to index heartbeat: %w", err)
	}

	// logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, fmt.Sprintf("Finished processing Heartbeat: %s", hb.ServiceId)))
	return nil
}
