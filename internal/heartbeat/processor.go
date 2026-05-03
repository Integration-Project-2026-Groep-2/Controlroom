package heartbeat

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

	var hb gen.Heartbeat

	if err := xml.Unmarshal(body, &hb); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("Unmarshal error when trying to unmarshal heartbeat xml: %v", err.Error())))
		return fmt.Errorf("Unmarshal error when trying to unmarshal heartbeat xml: %v", err.Error())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := indexHeartbeat(es, ctx, &hb); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("Failed to index user object: %s", err.Error())))
		return fmt.Errorf("Failed to index heartbeat: %s", err.Error())
	}

	logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, fmt.Sprintf("Finished processing Heartbeat: %s", hb.ServiceId)))
	return nil
}
