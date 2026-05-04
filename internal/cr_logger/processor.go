package cr_logger

import (
	"encoding/json"
	"fmt"

	"github.com/elastic/go-elasticsearch/v9"
	"integration-project-ehb/controlroom/pkg/logger"
)

type rawLogBody struct {
	Level   string `json:"level"`
	Service string `json:"service"`
	Msg     string `json:"msg"`
}

func ProcessLog(_ *elasticsearch.Client, body []byte) error {

	var raw rawLogBody

	err := json.Unmarshal(body, &raw)

	fmt.Println("DEBUGGING: ", raw)
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("unparseable log body: %s", body)))
		return fmt.Errorf("Failed to process Log: %s", err.Error())
	}

	logger.Log(logger.NewMessage(logger.Severity(raw.Level), logger.Service(raw.Service), raw.Msg))

	return nil
}
