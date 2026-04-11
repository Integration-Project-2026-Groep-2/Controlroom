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

// List of services we expect to see
var monitoredServices = []string{"DUMMY_CRM_SERVICE", "DUMMY_FRONTEND_SERVICE"}

// State tracker: true = ONLINE, false = OFFLINE
var serviceState = make(map[string]bool)

// The FIFO Queue for alerts (Buffer of 50 messages)
var alertQueue = make(chan string, 50)

// YOUR WEBHOOK URL GOES HERE
var teamsWebhookURL = os.Getenv("TEAMS_WEBHOOK_URL")

func main() {
	// Initialize all services as ONLINE
	for _, svc := range monitoredServices {
		serviceState[svc] = true
	}

	// 1. Start the Background Alert Worker (This handles the FIFO queue)
	go processAlertQueue()

	cfg := elasticsearch.Config{
		Addresses: []string{"http://localhost:9200"},
		Username:  "elastic",
		Password:  os.Getenv("ELASTIC_PASSWORD"),
	}
	es, err := elasticsearch.NewClient(cfg)
	if err != nil {
		log.Fatalf("ES Client error: %v", err)
	}

	fmt.Println("🐕 Watchdog started! Checking heartbeats every 10 seconds...")
	ticker := time.NewTicker(10 * time.Second)

	for range ticker.C {
		checkHeartbeats(es)
	}
}

// processAlertQueue runs endlessly in the background.
// It pulls one message from the queue, sends it, and waits 5 seconds.
func processAlertQueue() {
	for message := range alertQueue {
		sendTeamsAlert(message)
		// Crucial: Sleep to prevent Microsoft from rate-limiting us
		time.Sleep(5 * time.Second)
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
	if err != nil || res.IsError() {
		log.Printf("Error querying ES: %v", err)
		return
	}
	defer res.Body.Close()

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
			// Send to Queue instead of directly to Teams!
			alertQueue <- fmt.Sprintf("✅ **RESOLVED:** Service **%s** is back online!", svc)

		} else if !isCurrentlyOnline && wasOnline {
			serviceState[svc] = false
			log.Printf("🚨 %s is OFFLINE!", svc)
			// Send to Queue instead of directly to Teams!
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
