package analyser

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"integration-project-ehb/controlroom/pkg/gen"
	"integration-project-ehb/controlroom/pkg/logger"
	"io"
	"log"
	"strings"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/mailru/easyjson"
)

// struct used to save the amount of found warnings
type SimpleCount struct {
	Count int `json:"count"`
}

func CheckWarnings(es *elasticsearch.Client) ([]string, error) {
	//NOTE(Steven): When service names get replaced by env variables, other ways should be found to exclude. Also env file?
	// use all services, excluding controlroom and watchdog
	services := [6]string{"CRM", "KASSA", "FACTURATIE", "MAILING", "FRONTEND", "PLANNING"}
	warnings := []string{}
	for _, value := range services {
		warningSize, err := queryWarningAmount(es, value)
		if err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("%v", err)))
		}
		if warningSize >= 5 {
			warningXML, err := queryWarnings(es, value, warningSize)
			if err != nil {
				logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("%v", err)))
				return nil, err
			}
			warnings = append(warnings, warningXML)
		}
	}
	return warnings, nil
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

func queryWarningAmount(es *elasticsearch.Client, service string) (int, error) {
	var buf strings.Builder = *createquery(service)

	res, err := es.Count(
		es.Count.WithContext(context.Background()),
		es.Count.WithIndex("controlroom-logs"),
		es.Count.WithBody(strings.NewReader(buf.String())),
	)

	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("Warning check failed: %v", err)))
		return 0, err
	}

	var c SimpleCount
	json.NewDecoder(res.Body).Decode(&c)

	amountFound := c.Count

	res.Body.Close()

	return amountFound, nil
}

func queryWarnings(es *elasticsearch.Client, service string, amount int) (string, error) {
	var buf strings.Builder = *createquery(service)

	res, err := es.Search(
		es.Search.WithContext(context.Background()),
		es.Search.WithIndex("controlroom-logs"),
		es.Search.WithBody(strings.NewReader(buf.String())),
		es.Search.WithTrackTotalHits(true),
		es.Search.WithPretty(),
	)
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("Couldn't get warnings for service %s", service)))
		return "", err
	}

	data, err := io.ReadAll(res.Body)
	if err != nil {
		log.Fatalf("Error reading body: %s", err)
		return "", err
	}
	var response gen.ESWarningResponse
	err = easyjson.Unmarshal(data, &response)
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("Couldn't convert warning response json for %s into a struct.", service)))
		return "", err
	}

	var message gen.Warning

	message.Service = service
	message.Warnings = make([]gen.Warningdata, amount)

	for index, value := range response.Hits.Hits {
		var currentWarning gen.Warningdata
		currentWarning.Timestamp = value.Source.Timestamp
		currentWarning.Issue = value.Source.Data

		message.Warnings[index] = currentWarning
	}

	bytes, err := xml.MarshalIndent(message, "", "  ")
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, "Couldn't convert the warning struct to an xml file"))
		return "", err
	}

	// 2. Prepend the header and convert to string
	xmlString := xml.Header + string(bytes)
	return xmlString, nil
}
