package checkin

import (
	"context"
	"encoding/xml"
	"fmt"
	"time"

	"integration-project-ehb/controlroom/pkg/gen"

	"integration-project-ehb/controlroom/pkg/logger"

	"github.com/elastic/go-elasticsearch/v9"
)

// ProcessCheckin function handles the checkins from event visitors. this is received from the iot badge scanner
func ProcessCheckin(es *elasticsearch.Client, body []byte) error {

	var ci gen.CheckIn

	if err := xml.Unmarshal(body, &ci); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("checkin: failed to unmarshal XML: %v", err)))
		return fmt.Errorf("checkin: failed to unmarshal XML: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := indexCheckIn(es, ctx, &ci); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("checkin: failed to index checkin %s: %v", ci.Id, err)))
		return fmt.Errorf("checkin: failed to index checkin: %w", err)
	}

	return nil
}
