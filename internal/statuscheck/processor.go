package statuscheck

import (
	"context"
	"encoding/xml"
	"fmt"
	"time"

	"github.com/elastic/go-elasticsearch/v9"

	"integration-project-ehb/controlroom/pkg/gen"
	"integration-project-ehb/controlroom/pkg/logger"
)

func ProcessStatusCheck(es *elasticsearch.Client, body []byte) error {
	var sct gen.StatusCheck
	if err := xml.Unmarshal(body, &sct); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("Unmarshal error when trying to unmarshal statuscheck xml: %v", err.Error())))
		return fmt.Errorf("unmarshal: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := indexStatusCheck(es, ctx, &sct); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("Failed to index statuscheck: %s", err.Error())))
		return fmt.Errorf("index: %w", err)
	}

	logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, fmt.Sprintf("Indexed user object: %s", sct.ServiceId)))
	return nil
}
