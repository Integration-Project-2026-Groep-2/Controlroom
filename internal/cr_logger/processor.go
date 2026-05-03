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

// NOTE(nasr): unused elastic search client, because this isnt needed for the logging system.
// it uses a differnet logger reference in "integration-project-ehb/controlroom/pkg/logger"
// we added the parameter so we can keep the same function signature and use a function
// pointer in the main entry point
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
