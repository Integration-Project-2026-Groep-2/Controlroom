package checkin

import (
	"context"
	"encoding/xml"
	"fmt"
	"time"

	"integration-project-ehb/controlroom/pkg/gen"

	"github.com/elastic/go-elasticsearch/v9"
	"integration-project-ehb/controlroom/pkg/logger"
)

func ProcessHeartbeat(es *elasticsearch.Client, body []byte) error {

	var ci gen.CheckIn

	if err := xml.Unmarshal(body, &ci); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("Unmarshal error when trying to unmarshal heartbeat xml: %v", err.Error())))
		return fmt.Errorf("Unmarshal error when trying to unmarshal heartbeat xml: %v", err.Error())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := indexCheckIn(es, ctx, &ci); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("Failed to index user object: %s", err.Error())))
		return fmt.Errorf("Failed to index heartbeat: %s", err.Error())
	}

	return nil
}
