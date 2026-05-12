package warning_producer

import (
	"context"
	"encoding/json"
	"fmt"
	"integration-project-ehb/controlroom/cmd/config"
	"integration-project-ehb/controlroom/pkg/analyser"
	"integration-project-ehb/controlroom/pkg/logger"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	amqp "github.com/rabbitmq/amqp091-go"
)

func RunWarningProducer(es *elasticsearch.Client, ctx context.Context, ch *amqp.Channel) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			warnings, err := analyser.CheckWarnings(es)
			if err != nil {
				logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("failed to fetch data: %v", err)))
			}

			if len(warnings) > 0 {
				for _, value := range warnings {
					// Publish to the exchange
					err = publishMessage(ctx, ch, value)

					if err != nil {
						logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("failed to publish news: %v", err)))
					} else {
						logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, "published news warning"))
					}
				}
			}
			analyseCPU(es)
		}
	}
}

func publishMessage(ctx context.Context, ch *amqp.Channel, body string) error {
	err := ch.PublishWithContext(ctx,
		"news.topic",   // exchange
		"news.warning", // routing key (matches your setup() binding)
		false,
		false,
		amqp.Publishing{
			ContentType: "text/plain",
			Body:        []byte(body),
		})
	return err
}

func analyseCPU(es *elasticsearch.Client) {
	/*var baseQuery string = `{
	"size": 5,

	"query": {
		"bool": {
			"must": [
				{"wildcard": {"service_id": {"value": "%s*"}}},
				{"range": {"timestamp": {"gte": "now-10m"}}},
				{"range": {"cpu": {"gte": 0.75}}}
			]
		}
	},
	"sort": [
		{"timestamp": {"order": "desc"}}
	]
	}`
	*/
	var countQuery string = `{
		"query": {
			"bool": {
				"must": [
					{"wildcard": {"service_id": {"value": "%s*"}}},
					{"range": {"timestamp": {"gte": "now-10m"}}},
					{"range": {"cpu": {"gte": 0.75}}}
				]
			}
		}
	}`
	for _, val := range config.Services {
		//var query string = fmt.Sprintf(baseQuery, strings.ToLower(val))
		var buf strings.Builder
		buf.WriteString(fmt.Sprintf(countQuery, strings.ToLower(val)))

		res, err := es.Count(
			es.Count.WithContext(context.Background()),
			es.Count.WithIndex("statuscheck"),
			es.Count.WithBody(strings.NewReader(buf.String())),
		)

		if err != nil {
			logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, fmt.Sprintf("failed to connect to statuscheck: %v", err)))
		}

		var count int

		json.NewDecoder(res.Body).Decode(&count)

		res.Body.Close()
	}
}
