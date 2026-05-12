package integration_tests

import (
	"fmt"
	"os"
	"time"

	"integration-project-ehb/controlroom/pkg/logger"

	"github.com/elastic/go-elasticsearch/v9"
)

func send() {
	cfg := elasticsearch.Config{
		Addresses: []string{os.Getenv("ELASTICSEARCH_URL")},
		Username:  os.Getenv("CONTROLROOM_ES_USER"),
		Password:  os.Getenv("CONTROLROOM_ES_PASS"),
	}

	if err := logger.Init(&cfg, "controlroom-logs", os.Stdout, 4); err != nil {
		fmt.Fprintf(os.Stderr, "logger init: %v\n", err)
		os.Exit(4)
	}
	defer logger.Shutdown()

	for range 5 {
		logger.Log(logger.NewMessage(logger.WARN, logger.KASSA, fmt.Sprintf("Test error lols")))
	}
	time.Sleep(2 * time.Second)
}
