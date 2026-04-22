package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	elasticsearch "github.com/elastic/go-elasticsearch/v9"
)

// Configuratie uit .env
var monitoredServices = strings.Split(os.Getenv("MONITORED_SERVICES"), ",")
var teamsWebhookURL = os.Getenv("TEAMS_WEBHOOK_URL")

// State trackers
var serviceState = make(map[string]bool)
var esOnline = true // We gaan ervan uit dat ES in het begin online is

// The FIFO Queue for alerts (Buffer of 50 messages)
var alertQueue = make(chan string, 50)

func main() {
	if teamsWebhookURL == "" {
		log.Fatal("🚨 CRITICAL: TEAMS_WEBHOOK_URL is niet ingesteld in de environment!")
	}

	for _, svc := range monitoredServices {
		serviceState[svc] = true
	}

	go processAlertQueue()

	esURL := os.Getenv("ELASTICSEARCH_URL")
	cfg := elasticsearch.Config{
		Addresses: []string{esURL},
		Username:  os.Getenv("WATCHDOG_ES_USER"),
		Password:  os.Getenv("WATCHDOG_ES_PASS"),
	}
	es, err := elasticsearch.NewClient(cfg)
	if err != nil {
		log.Fatalf("ES Client config error: %v", err)
	}

	fmt.Println("🐕 Watchdog started! Checking heartbeats every 60 seconds...")
	// Ticker aangepast naar 60 seconden!
	ticker := time.NewTicker(60 * time.Second)

	for range ticker.C {
		checkHeartbeats(es)
	}
}

// processAlertQueue runs endlessly in the background.
func processAlertQueue() {
	for message := range alertQueue {
		sendTeamsAlert(message)
		time.Sleep(6 * time.Second)
	}
}

func checkHeartbeats(es *elasticsearch.Client) {
	query := `{
		"size": 0,
		"query": { "range": { "timestamp": { "gte": "now-60s" } } },
		"aggs": { "services": { "terms": { "field": "serviceId.keyword", "size": 100 } } }
	}`

	res, err := es.Search(
		es.Search.WithIndex("heartbeats"),
		es.Search.WithBody(strings.NewReader(query)),
	)

	// SCENARIO 1: ELASTICSEARCH IS ONBEREIKBAAR (Netwerk Error / Server Plat)
	if err != nil {
		if esOnline {
			esOnline = false
			log.Printf("🚨 Netwerkfout: Elasticsearch onbereikbaar: %v", err)
			alertQueue <- "🚨 **CRITICAL:** Elasticsearch is onbereikbaar! Watchdog kan momenteel geen services controleren."
		}
		return // Sla service checks over
	}

	// SCENARIO 2: ELASTICSEARCH KOMT NET TERUG ONLINE NA EEN UITVAL
	if !esOnline {
		esOnline = true
		log.Println("✅ Elasticsearch is terug online! Services worden bij de volgende check (over 60s) weer gecontroleerd.")
		alertQueue <- "✅ **RESOLVED:** Elasticsearch is terug bereikbaar! Watchdog wacht één cyclus om heartbeats de kans te geven binnen te komen."
		if res != nil {
			res.Body.Close()
		}
		return // Belangrijk: We slaan de check NU over. Bij de volgende 'tick' (over 60 seconden) checkt hij pas weer!
	}

	// SCENARIO 3: NETWERK IS OKÉ, MAAR HTTP ERROR (bijv. 401 Unauthorized / 403 Forbidden)
	if res.IsError() {
		if res.StatusCode == 401 || res.StatusCode == 403 {
			log.Printf("🔒 Authenticatie fout (%d): Geen toegang. Wachten tot account in Kibana is aangemaakt.", res.StatusCode)
		} else {
			log.Printf("⚠️ Elasticsearch foutmelding: %s", res.String())
		}
		res.Body.Close()
		return // Sla service checks over, stuur GEEN Teams alert!
	}

	defer res.Body.Close()

	// SCENARIO 4: ALLES IS NORMAAL, CHECK DE SERVICES
	var result map[string]interface{}
	json.NewDecoder(res.Body).Decode(&result)

	counts := make(map[string]float64)
	if aggregations, ok := result["aggregations"].(map[string]interface{}); ok {
		if services, ok := aggregations["services"].(map[string]interface{}); ok {
			if buckets, ok := services["buckets"].([]interface{}); ok {
				for _, b := range buckets {
					bucket := b.(map[string]interface{})
					key := bucket["key"].(string)
					counts[key] = bucket["doc_count"].(float64)
				}
			}
		}
	}

	for _, svc := range monitoredServices {
		count := counts[svc]
		isCurrentlyOnline := count >= 55
		wasOnline := serviceState[svc]

		if isCurrentlyOnline && !wasOnline {
			serviceState[svc] = true
			log.Printf("✅ %s is BACK ONLINE!", svc)
			alertQueue <- fmt.Sprintf("✅ **RESOLVED:** Service **%s** is back online!", svc)
		} else if !isCurrentlyOnline && wasOnline {
			serviceState[svc] = false
			log.Printf("🚨 %s is OFFLINE!", svc)
			alertQueue <- fmt.Sprintf("🚨 **CRITICAL:** Service **%s** is down! (Heartbeats in last 60s: %v)", svc, count)
		}
	}
}

func sendTeamsAlert(message string) {
	payload := map[string]interface{}{
		"type": "message",
		"attachments": []map[string]interface{}{
			{
				"contentType": "application/vnd.microsoft.card.adaptive",
				"content": map[string]interface{}{
					"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
					"type":    "AdaptiveCard",
					"version": "1.2",
					"body": []map[string]interface{}{
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
	resp, err := http.Post(teamsWebhookURL, "application/json", bytes.NewBuffer(jsonValue))

	if err != nil {
		log.Printf("Failed to send Teams alert: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		buf := new(bytes.Buffer)
		buf.ReadFrom(resp.Body)
		log.Printf("Teams webhook rejected the payload. HTTP Status: %v. Response: %s", resp.StatusCode, buf.String())
	}
}
