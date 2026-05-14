package watchdog

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
	"time"

	"integration-project-ehb/controlroom/cmd/config"
	"integration-project-ehb/controlroom/pkg/gen"
	"integration-project-ehb/controlroom/pkg/logger"

	"github.com/elastic/go-elasticsearch/v9"
	amqp "github.com/rabbitmq/amqp091-go"
)

func publishHeartbeatStatus(svc string, count float64, up bool, severity SeverityLevel, event EventType) {

	ch := WatchdogChan.Load()

	if ch == nil {
		logger.Log(logger.NewMessage(logger.WARN, logger.WATCHDOG, "RabbitMQ channel is not initialized"))
		return
	}

	summary := fmt.Sprintf("%s heartbeat is back online", svc)

	if !up {
		summary = fmt.Sprintf("%s heartbeat missed (count %v in last 60s)", svc, count)
	}

	var buf bytes.Buffer
	var body = gen.HeartbeatStatusEventType{

		Event:     string(event),
		Source:    "controlroom-watchdog",
		Timestamp: time.Now().UTC(),
		Payload: gen.HeartbeatPayloadType{
			Summary:   summary,
			Severity:  string(severity),
			Component: strings.ToLower(svc),
			Group:     "services",
			Class:     "heartbeat-loss",
			CustomDetails: gen.HeartbeatCustomDetailsType{
				HeartbeatCountLast60s: count,
				Threshold:             30,
				LastCheckAt:           time.Now().UTC(),
			},
		},
	}

	enc := xml.NewEncoder(&buf)
	if err := enc.Encode(body); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.WATCHDOG, fmt.Sprintf("failed to encode to xml: %v", err)))
		return
	}

	if err := enc.Flush(); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.WATCHDOG, fmt.Sprintf("failed to encode to xml, (flush thing): %v", err)))
	}

	if err := ch.PublishWithContext(
		context.Background(),
		config.Producer[config.HEARTBEAT_SUCCEEDED_EVENT].Exchange.Name,
		config.Producer[config.HEARTBEAT_SUCCEEDED_EVENT].Key.Key,
		false,
		false,
		amqp.Publishing{
			ContentType: "application/xml",
			Body:        buf.Bytes(),
		},
	); err != nil {
		logger.Log(logger.NewMessage(logger.WARN, logger.WATCHDOG, fmt.Sprintf("publish heartbeat event failed: %v", err)))
	}

}

func CheckHeartbeats(client *elasticsearch.Client) {
	counts := make(map[string]float64)

	for _, svc := range Services {
		svcLower := strings.ToLower(svc)

		// Create a proper Elasticsearch query structure
		query := fmt.Sprintf(`{
			"query": {
				"bool": {
					"must": [
						{ "match": { "service.name": "%s" } },
						{ "range": { "@timestamp": { "gte": "now-60s" } } }
					]
				}
			}
		}`, svcLower)

		res, err := client.Search(
			client.Search.WithIndex("heartbeats"),
			client.Search.WithBody(strings.NewReader(query)),
		)
		if err != nil {
			logger.Log(logger.NewMessage(logger.WARN, logger.WATCHDOG, fmt.Sprintf("Failed to query elastic (%s): %v", svc, err)))
			continue
		}

		if res.IsError() {
			if res.StatusCode == 401 || res.StatusCode == 403 {
				logger.Log(logger.NewMessage(logger.ERROR, logger.WATCHDOG, fmt.Sprintf("Authenticatie fout (%d): Geen toegang. Wachten tot account in Kibana is aangemaakt.", res.StatusCode)))
			} else {
				logger.Log(logger.NewMessage(logger.WARN, logger.WATCHDOG, fmt.Sprintf("Elasticsearch error: %s", res.String())))
			}
			res.Body.Close()
			continue
		}

		var result map[string]any
		if err := json.NewDecoder(res.Body).Decode(&result); err == nil {
			if hits, ok := result["hits"].(map[string]any); ok {
				if total, ok := hits["total"].(map[string]any); ok {
					if value, ok := total["value"].(float64); ok {
						counts[strings.ToUpper(svc)] = value
					}
				}
			}
		}
		res.Body.Close()
	}

	StateMutex.Lock()
	defer StateMutex.Unlock()

	for _, svc := range Services {
		count := counts[svc]
		isCurrentlyOnline := count >= 30
		wasOnline := ServiceState[svc]

		if isCurrentlyOnline && !wasOnline {
			ServiceState[svc] = true
			logger.Log(logger.NewMessage(logger.INFO, logger.WATCHDOG, fmt.Sprintf("%s is ONLINE!", svc)))

			publishHeartbeatStatus(svc, count, true, Info, HeartbeatOnline)
			WatchdogQueue <- fmt.Sprintf("**RESOLVED:** Service **%s** is back online!", svc)

		} else if !isCurrentlyOnline && wasOnline {
			ServiceState[svc] = false
			logger.Log(logger.NewMessage(logger.WARN, logger.WATCHDOG, fmt.Sprintf("%s is OFFLINE!", svc)))

			// Passed all 5 required arguments
			publishHeartbeatStatus(svc, count, false, Critical, HeartbeatFailed)
			WatchdogQueue <- fmt.Sprintf("**CRITICAL:** Service **%s** is down! (Heartbeats in last 60s: %v)", svc, count)
		}
	}
}

func AlertTeams(message string) {
	if TeamsWebhook == "" {
		logger.Log(logger.NewMessage(logger.WARN, logger.WATCHDOG, "Teams webhook URL is not configured"))
		return
	}

	data := TeamsAlertCard{
		Type: "message",
		Attachments: []TeamAttachment{
			{
				ContentType: "application/vnd.microsoft.card.adaptive",
				Content: &AdaptiveCard{
					Schema:  "http://adaptivecards.io/schemas/adaptive-card.json",
					Type:    "AdaptiveCard",
					Version: "1.2",
					Body: []TextBlock{
						{
							Type: "TextBlock",
							Text: message,
							Wrap: true,
						},
					},
				},
			},
		},
	}

	body, err := json.Marshal(data)
	if err != nil {
		logger.Log(logger.NewMessage(logger.WARN, logger.WATCHDOG, err.Error()))
		return
	}

	resp, err := http.Post(TeamsWebhook, "application/json", bytes.NewBuffer(body))
	if err != nil {
		logger.Log(logger.NewMessage(logger.WARN, logger.WATCHDOG, err.Error()))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		buf := new(bytes.Buffer)
		buf.ReadFrom(resp.Body)
		logger.Log(logger.NewMessage(logger.WARN, logger.WATCHDOG, fmt.Sprintf("teams webhook rejected the payload. status: %d, response: %s", resp.StatusCode, buf.String())))
	}
}
