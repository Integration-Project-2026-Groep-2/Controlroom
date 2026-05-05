package analyser

import (
	"context"
	"fmt"
	"integration-project-ehb/controlroom/pkg/logger"
	"strings"

	"github.com/elastic/go-elasticsearch/v9"
)

func CheckWarnings(es *elasticsearch.Client) {
	severity := logger.WARN

	var buf strings.Builder
	query := fmt.Sprintf(`{
	  "query": {
	    "term": {
	      "level.keyword": "%v"
	    }
	  }
	}`, severity)

	buf.WriteString(query)

	res, err := es.Search(
		es.Search.WithContext(context.Background()),
		es.Search.WithIndex("controlroom-logs"),
		es.Search.WithBody(strings.NewReader(buf.String())),
		es.Search.WithTrackTotalHits(true),
		es.Search.WithPretty(),
	)
	defer res.Body.Close()

	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("Warning check failed: %v", err)))
	}

	fmt.Println(res)
}
