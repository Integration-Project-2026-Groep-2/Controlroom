// AI generated test
package main

/*

	# Used for logger tests.
  #Used for the simple reason that I cannot get it to work outside the controlroom-network

*/

import (
	"fmt"
	"os"
	"time"

	"integration-project-ehb/controlroom/pkg/logger"

	"github.com/elastic/go-elasticsearch/v9"
)

func main() {
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

	logger.Log(logger.NewMessage(logger.WARN, logger.KASSA, fmt.Sprintf("Test error lols")))

	// 4. Wait a moment for the background worker to process the queue
	fmt.Println("Waiting for Elasticsearch indexing...")
	time.Sleep(2 * time.Second)
	fmt.Println("Done.")
}
