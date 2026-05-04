package cr_logger

import (
	"encoding/json"
	"fmt"
	"github.com/elastic/go-elasticsearch/v9"
	"integration-project-ehb/controlroom/pkg/gen"
	"integration-project-ehb/controlroom/pkg/logger"
)

// ProcessLog unmarshals a LogEvent from RabbitMQ and indexes it
func ProcessLog(_ *elasticsearch.Client, body []byte) error {
	var event gen.LogEvent
	err := json.Unmarshal(body, &event)
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("unparseable log body: %s", body)))
		return fmt.Errorf("Failed to process Log: %s", err.Error())
	}

	// Map gen.SeverityType to logger.Severity
	logger.Log(logger.NewMessage(logger.Severity(event.Level), logger.Service(event.Service), event.Data))
	return nil
}
