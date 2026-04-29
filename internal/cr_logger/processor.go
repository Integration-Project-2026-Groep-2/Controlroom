package cr_logger

import (
	"encoding/json"
	"fmt"

	"integration-project-ehb/controlroom/pkg/logger"
	"github.com/elastic/go-elasticsearch/v9"
)

type rawLogBody struct {
	Level   string `json:"level"`
	Service string `json:"service"`
	Msg     string `json:"msg"`
}

func ProcessLog(_ *elasticsearch.Client, body []byte) error {

	var raw rawLogBody

	if err := json.Unmarshal(body, &raw); err != nil {
		logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM,
			fmt.Sprintf("unparseable log body: %s", body)))
		return nil // don't DLQ on parse failure
	}

	logger.Log(logger.NewMessage(
		logger.Severity(raw.Level),
		logger.Service(raw.Service),
		raw.Msg,
	))
	return nil
}
