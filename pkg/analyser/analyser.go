package analyser

import (
	"context"
	"encoding/json"
	"fmt"
	"integration-project-ehb/controlroom/pkg/logger"
	"strings"

	"github.com/elastic/go-elasticsearch/v9"
)

// struct used to save the amount of found warnings
type SimpleCount struct {
	Count int `json:"count"`
}

func CheckWarnings(es *elasticsearch.Client) {
	//NOTE(Steven): When service names get replaced by env variables, other ways should be found to exclude. Also env file?
	// use all services, excluding controlroom and watchdog
	services := [6]string{"CRM", "KASSA", "FACTURATIE", "MAILING", "FRONTEND", "PLANNING"}
	for _, value := range services {
		queryWarnings(es, value)
	}
}

func createquery(service string) *strings.Builder {
	var useService string = fmt.Sprintf(`{ "term": { "service.keyword": "%s" } }`, service)
	var buf strings.Builder
	query := fmt.Sprintf(`{
			"query": {
				"bool": {
				"must": [
					{ "term": { "level.keyword": "WARN" } },
					%s
				],
				"must_not": [
					
				]
				}
			}
			}`, useService)
	buf.WriteString(query)
	return &buf
}

func queryWarnings(es *elasticsearch.Client, service string) {
	var buf strings.Builder = *createquery(service)

	res, err := es.Count(
		es.Count.WithContext(context.Background()),
		es.Count.WithIndex("controlroom-logs"),
		es.Count.WithBody(strings.NewReader(buf.String())),
	)

	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("Warning check failed: %v", err)))
	}

	var c SimpleCount
	json.NewDecoder(res.Body).Decode(&c)

	res.Body.Close()

	if c.Count >= 5 {
		res, err := es.Search(
			es.Search.WithContext(context.Background()),
			es.Search.WithIndex("controlroom-logs"),
			es.Search.WithBody(strings.NewReader(buf.String())),
			es.Search.WithTrackTotalHits(true),
			es.Search.WithPretty(),
		)
		if err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("Couldn't get warnings for service %s", service)))
		}
		fmt.Println(res)
	}
}
