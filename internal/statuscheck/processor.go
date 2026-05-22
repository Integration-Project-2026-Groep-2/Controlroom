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
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("statuscheck: failed to unmarshal XML: %v", err)))
		return fmt.Errorf("statuscheck: failed to unmarshal XML: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := indexStatusCheck(es, ctx, &sct); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("statuscheck: failed to index statuscheck for %s: %v", sct.ServiceId, err)))
		return fmt.Errorf("statuscheck: failed to index statuscheck: %w", err)
	}

	// logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, fmt.Sprintf("Finished processing StatusCheck: %s", sct.ServiceId)))
	return nil
}
