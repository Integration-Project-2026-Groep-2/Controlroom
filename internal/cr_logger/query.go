package cr_logger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	config "integration-project-ehb/controlroom/internal/cr_config"
	"integration-project-ehb/controlroom/pkg/gen"
	"integration-project-ehb/controlroom/pkg/logger"

	"github.com/elastic/go-elasticsearch/v9"
)

func QueryAll(es *elasticsearch.Client) ([]gen.Warning, error) {
	var results []gen.Warning

	for _, service := range config.Services {
		query := fmt.Sprintf(`{
			"query": {
				"bool": {
					"must": [
						{ "term": { "service.keyword": "%s" } }
					]
				}
			}
		}`, service)

		res, err := es.Search(
			es.Search.WithContext(context.Background()),
			es.Search.WithIndex("controlroom-logs"),
			es.Search.WithBody(strings.NewReader(query)),
		)

		if err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("cr_logger: failed to query logs for %s: %v", service, err)))
			continue
		}

		defer res.Body.Close()

		data, err := io.ReadAll(res.Body)
		if err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("cr_logger: failed to read log response body: %v", err)))
			continue
		}

		var response gen.ESWarningResponse
		if err := json.Unmarshal(data, &response); err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("cr_logger: failed to unmarshal log response: %v", err)))
			continue
		}

		message := gen.Warning{
			Service:  service,
			Warnings: make([]gen.Warningdata, 0, len(response.Hits.Hits)),
		}

		for _, hit := range response.Hits.Hits {
			message.Warnings = append(message.Warnings, gen.Warningdata{
				Timestamp: hit.Source.Timestamp,
				Issue:     hit.Source.Data,
			})
		}

		results = append(results, message)
	}

	return results, nil
}

func QueryWarning(es *elasticsearch.Client) ([]gen.Warning, error) {
	var results []gen.Warning

	for _, service := range config.Services {
		query := fmt.Sprintf(`{
			"query": {
				"bool": {
					"must": [
						{ "term": { "level.keyword": "WARN" } },
						{ "term": { "service.keyword": "%s" } }
					]
				}
			}
		}`, service)

		res, err := es.Search(
			es.Search.WithContext(context.Background()),
			es.Search.WithIndex("controlroom-logs"),
			es.Search.WithBody(strings.NewReader(query)),
		)
		if err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("cr_logger: failed to query warnings for %s: %v", service, err)))
			continue
		}
		defer res.Body.Close()

		data, err := io.ReadAll(res.Body)
		if err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("cr_logger: failed to read warning response body: %v", err)))
			continue
		}

		var response gen.ESWarningResponse
		if err := json.Unmarshal(data, &response); err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("cr_logger: failed to unmarshal warning response: %v", err)))
			continue
		}

		message := gen.Warning{
			Service:  service,
			Warnings: make([]gen.Warningdata, 0, len(response.Hits.Hits)),
		}

		for _, hit := range response.Hits.Hits {
			message.Warnings = append(message.Warnings, gen.Warningdata{
				Timestamp: hit.Source.Timestamp,
				Issue:     hit.Source.Data,
			})
		}

		results = append(results, message)
	}

	return results, nil
}

func QueryError(es *elasticsearch.Client) ([]gen.WatchdogError, error) {
	var results []gen.WatchdogError

	for _, service := range config.Services {
		query := fmt.Sprintf(`{
			"query": {
				"bool": {
					"must": [
						{ "term": { "level.keyword": "ERROR" } },
						{ "term": { "service.keyword": "%s" } }
					]
				}
			}
		}`, service)

		res, err := es.Search(
			es.Search.WithContext(context.Background()),
			es.Search.WithIndex("controlroom-logs"),
			es.Search.WithBody(strings.NewReader(query)),
		)
		if err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("cr_logger: failed to query errors for %s: %v", service, err)))
			continue
		}
		defer res.Body.Close()

		data, err := io.ReadAll(res.Body)
		if err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("cr_logger: failed to read error response body: %v", err)))
			continue
		}

		var response gen.ESWarningResponse
		if err := json.Unmarshal(data, &response); err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("cr_logger: failed to unmarshal error response: %v", err)))
			continue
		}

		message := gen.WatchdogError{
			Service: service,
			Errors:  make([]gen.WatchdogErrorData, 0, len(response.Hits.Hits)),
		}

		for _, hit := range response.Hits.Hits {
			message.Errors = append(message.Errors, gen.WatchdogErrorData{
				Timestamp: hit.Source.Timestamp,
				Issue:     hit.Source.Data,
			})
		}

		results = append(results, message)
	}

	return results, nil
}
