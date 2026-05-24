package jarvis_metrics

import (
	"context"
	"fmt"
	config "integration-project-ehb/controlroom/internal/cr_config"
	"integration-project-ehb/controlroom/pkg/logger"
	"net/http"

	"github.com/elastic/go-elasticsearch/v9"
)

func ProcessJarvisMetrics(ctx context.Context, es *elasticsearch.Client, httpClient *http.Client) {

	logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, "[DEBUG] indexing metrics"))

	data, err := RetrieveMetrics(ctx, httpClient, config.McpMasterUrl)
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.MCP, fmt.Sprintf("failed to retrieve jarvis statistics: %s", err)))
		return
	}
	err = IndexMetrics(es, ctx, data)
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("failed to retrieve jarvis statistics: %s", err)))
	}

	logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, fmt.Sprintf("mission succesful jarvis, let's head back home!")))
}
