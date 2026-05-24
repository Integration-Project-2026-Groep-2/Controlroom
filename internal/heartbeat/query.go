package heartbeat

import (
	"context"
	"fmt"
	"strings"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"
	"integration-project-ehb/controlroom/pkg/logger"
)

func QueryRecentHeartbeats(ctx context.Context, es *elasticsearch.Client, service string, pageSize, withinSeconds int) (*esapi.Response, error) {
	sId := strings.ToLower(service)
	body := fmt.Sprintf(`{
		"size": %d,
		"query": {
			"bool": {
				"must": [
					{ "term": { "serviceId": "%s" } },
					{ "range": { "timestamp": { "gte": "now-%ds" } } }
				]
			}
		}
	}`, pageSize, sId, withinSeconds)

	res, err := es.Search(
		es.Search.WithContext(ctx),
		es.Search.WithIndex("heartbeats"),
		es.Search.WithBody(strings.NewReader(body)),
	)
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM,
			fmt.Sprintf("QueryRecentHeartbeats failed for %s: %v", sId, err)))
		return nil, err
	}
	return res, nil
}

func QueryLastSeenPerService(ctx context.Context, es *elasticsearch.Client) (*esapi.Response, error) {
	body := ` {
	"size": 0,
	"query": {
		"range": {
			"timestamp": { "gte": "now-60s" }
    }
  },
  "aggs": {
    "per_service": {
      "terms": { "field": "serviceId" },
      "aggs": { "last_seen": { "max": { "field": "timestamp" } } } }
  }
}

	res, err := es.Search(
		es.Search.WithContext(ctx),
		es.Search.WithIndex("heartbeats"),
		es.Search.WithBody(strings.NewReader(body)),
	)
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM,
			fmt.Sprintf("QueryLastSeenPerService failed: %v", err)))
		return nil, err
	}
	return res, nil
}

func QuerySilentServices(ctx context.Context, es *elasticsearch.Client, withinSeconds int) (*esapi.Response, error) {
	body := fmt.Sprintf(`{
		"size": 0,
		"query": {
			"bool": {
				"must_not": [
					{ "range": { "timestamp": { "gte": "now-%ds" } } }
				]
			}
		},
		"aggs": {
			"silent_services": {
				"terms": { "field": "serviceId" }
			}
		}
	}`, withinSeconds)

	res, err := es.Search(
		es.Search.WithContext(ctx),
		es.Search.WithIndex("heartbeats"),
		es.Search.WithBody(strings.NewReader(body)),
	)
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM,
			fmt.Sprintf("QuerySilentServices failed: %v", err)))
		return nil, err
	}
	return res, nil
}
