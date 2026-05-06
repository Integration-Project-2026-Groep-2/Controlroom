package watchdog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"integration-project-ehb/controlroom/pkg/logger"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
)

var WDServices = [6]string{"CRM", "FACTURATIE", "FRONTEND", "MAILING", "PLANNING", "KASSA"} // State trackers
var WDServiceState = map[string]bool{
	"CRM":        false,
	"FACTURATIE": false,
	"FRONTEND":   false,
	"MAILING":    false,
	"PLANNING":   false,
	"KASSA":      false,
}

var WDQueue = make(chan string, 50) // The FIFO Queue for alerts (Buffer of 50 messages)
var WDWebhook = os.Getenv("TEAMS_WEBHOOK_URL")

// ProcessAlertQueue runs endlessly in the background.
func ProcessAlertQueue() {
	for message := range WDQueue {
		sendTeamsAlert(message)
		time.Sleep(6 * time.Second)
	}
}

func CheckHeartbeats(client *elasticsearch.Client) {
	counts := make(map[string]float64)
	for _, svc := range WDServices {
		svc = strings.ToLower(svc)
		query := `{
			"size": 0,
			"query": {
				"bool": {
				"filter": [
					{ "range": { "timestamp": { "gte": "now-60s" } } },
					{ "term": { "service_id.keyword": "SERVICE" } }
				]
				}
			}
		}`

		query = strings.Replace(query, "SERVICE", svc, 1)

		// TODO(nasr): error handling
		res, err := client.Search(
			client.Search.WithIndex("heartbeats"),
			client.Search.WithBody(strings.NewReader(query)),
		)
		if err != nil {
			logger.Log(logger.NewMessage(logger.WARN, logger.WATCHDOG, fmt.Sprintf("Failed to query elastic (%s): ", err)))

		}

		//  Netwerk is oké, maar http error (bijv. 401 unauthorized / 403 forbidden)
		if res.IsError() {
			if res.StatusCode == 401 || res.StatusCode == 403 {
				logger.Log(logger.NewMessage(logger.WARN, logger.WATCHDOG, fmt.Sprintf("Authenticatie fout (%d): Geen toegang. Wachten tot account in Kibana is aangemaakt.", res.StatusCode)))
			} else {
				logger.Log(logger.NewMessage(logger.ERROR, logger.WATCHDOG, fmt.Sprintf("%s is ONLINE!", res.String())))
			}

			res.Body.Close()
			return // Sla service checks over, stuur GEEN Teams alert!
		}

		// Alles is normaal, check de services
		var result map[string]any
		json.NewDecoder(res.Body).Decode(&result)

		if hits, ok := result["hits"].(map[string]any); ok {
			if total, ok := hits["total"].(map[string]any); ok {
				if value, ok := total["value"].(float64); ok {
					counts[strings.ToUpper(svc)] = value
				}
			}
		}
		res.Body.Close()
	}

	for _, svc := range WDServices {

		count := counts[svc]
		// heartbeat margin / minute
		isCurrentlyOnline := count >= 30
		wasOnline := WDServiceState[svc]

		if isCurrentlyOnline && !wasOnline {
			WDServiceState[svc] = true
			logger.Log(logger.NewMessage(logger.INFO, logger.WATCHDOG, fmt.Sprintf("%s is ONLINE!", svc)))
			WDQueue <- fmt.Sprintf("**RESOLVED:** Service **%s** is back online!", svc)

		} else if !isCurrentlyOnline && wasOnline {
			fmt.Println("")
			fmt.Printf("Service %s is pop at %v", svc, time.Now())
			fmt.Println("")
			WDServiceState[svc] = false
			logger.Log(logger.NewMessage(logger.WARN, logger.WATCHDOG, fmt.Sprintf("%s is OFFLINE!", svc)))
			WDQueue <- fmt.Sprintf("**CRITICAL:** Service **%s** is down! (Heartbeats in last 60s: %v)", svc, count)

		}
	}
}

func sendTeamsAlert(message string) {

	payload := map[string]any{
		"type": "message",
		"attachments": []map[string]any{
			{
				"contentType": "application/vnd.microsoft.card.adaptive",
				"content": map[string]any{
					"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
					"type":    "AdaptiveCard",
					"version": "1.2",
					"body": []map[string]any{
						{
							"type": "TextBlock",
							"text": message,
							"wrap": true,
						},
					},
				},
			},
		},
	}

	jsonValue, _ := json.Marshal(payload)
	resp, err := http.Post(WDWebhook, "application/json", bytes.NewBuffer(jsonValue))

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
