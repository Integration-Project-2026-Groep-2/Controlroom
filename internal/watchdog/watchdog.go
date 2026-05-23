package watchdog

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	config "integration-project-ehb/controlroom/internal/cr_config"
	"integration-project-ehb/controlroom/internal/cr_logger"
	"integration-project-ehb/controlroom/internal/heartbeat"
	"integration-project-ehb/controlroom/pkg/logger"
	"net/http"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	amqp "github.com/rabbitmq/amqp091-go"
)

const errors_threshold = 5
const warnings_threshold = 5

func RunWatchdog(es *elasticsearch.Client, ctx context.Context, ch *amqp.Channel) {
	tiktak := time.NewTicker(3600 * time.Second)
	defer tiktak.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tiktak.C:

			// warning checker
			{
				warnings, err := cr_logger.QueryWarning(es)
				if err != nil {
					logger.Log(logger.NewMessage(logger.ERROR, logger.WATCHDOG, fmt.Sprintf("watchdog: failed to fetch warnings: %v", err)))
					continue
				}

				for _, warning := range warnings {
					if len(warning.Warnings) > warnings_threshold {
						var buf bytes.Buffer
						enc := xml.NewEncoder(&buf)

						if err := enc.Encode(warning); err != nil {
							logger.Log(logger.NewMessage(logger.ERROR, logger.WATCHDOG, fmt.Sprintf("watchdog: failed to encode warning to XML: %v", err)))
							continue
						}

						if err := enc.Flush(); err != nil {
							logger.Log(logger.NewMessage(logger.ERROR, logger.WATCHDOG, fmt.Sprintf("watchdog: failed to flush warning XML encoder: %v", err)))
							continue
						}

						if err := ch.PublishWithContext(ctx,
							config.Producer[config.WARNING_EVENT].Exchange.Name,
							config.Producer[config.WARNING_EVENT].Exchange.Kind,
							false,
							false,
							amqp.Publishing{
								ContentType: "application/xml",
								Body:        buf.Bytes(),
							}); err != nil {
							logger.Log(logger.NewMessage(logger.ERROR, logger.WATCHDOG, fmt.Sprintf("watchdog: failed to publish warning: %v", err)))
						} else {
							logger.Log(logger.NewMessage(logger.DEBUG, logger.WATCHDOG, "watchdog: published warning"))
						}
					}
				}
			}

			// error checker
			{
				errors, err := cr_logger.QueryError(es)
				if err != nil {
					logger.Log(logger.NewMessage(logger.ERROR, logger.WATCHDOG, fmt.Sprintf("watchdog: failed to fetch errors: %v", err)))
					continue
				}

				for _, errData := range errors {
					if len(errData.Errors) > errors_threshold {
						var buf bytes.Buffer
						enc := xml.NewEncoder(&buf)

						if err := enc.Encode(errData); err != nil {
							logger.Log(logger.NewMessage(logger.ERROR, logger.WATCHDOG, fmt.Sprintf("watchdog: failed to encode error to XML: %v", err)))
							continue
						}

						if err := enc.Flush(); err != nil {
							logger.Log(logger.NewMessage(logger.ERROR, logger.WATCHDOG, fmt.Sprintf("watchdog: failed to flush error XML encoder: %v", err)))
							continue
						}

						if err := ch.PublishWithContext(ctx,
							config.Producer[config.ERROR_EVENT].Exchange.Name,
							config.Producer[config.ERROR_EVENT].Exchange.Kind,
							false,
							false,
							amqp.Publishing{
								ContentType: "application/xml",
								Body:        buf.Bytes(),
							}); err != nil {
							logger.Log(logger.NewMessage(logger.ERROR, logger.WATCHDOG, fmt.Sprintf("watchdog: failed to publish error: %v", err)))
						} else {
							logger.Log(logger.NewMessage(logger.DEBUG, logger.WATCHDOG, "watchdog: published error"))
						}
					}
				}
			}

			// heartbeat checker
			{
				resp, err := heartbeat.QueryLastSeenPerService(ctx, es)
				if err != nil {
					logger.Log(logger.NewMessage(logger.ERROR, logger.WATCHDOG, fmt.Sprintf("watchdog: failed to query heartbeat data: %v", err)))
					// Stop execution for this tick so we don't accidentally mark everything offline
					continue
				}

				defer resp.Body.Close()

				var esResult struct {
					Aggregations struct {
						Services struct {
							Buckets []struct {
								Key      string  `json:"key"`
								DocCount float64 `json:"doc_count"`
							} `json:"buckets"`
						} `json:"services"`
					} `json:"aggregations"`
				}

				if err := json.NewDecoder(resp.Body).Decode(&esResult); err != nil {
					logger.Log(logger.NewMessage(logger.ERROR, logger.WATCHDOG, fmt.Sprintf("watchdog: failed to decode heartbeat response: %v", err)))
					continue
				}

				counts := make(map[string]float64)
				for _, bucket := range esResult.Aggregations.Services.Buckets {
					counts[bucket.Key] = bucket.DocCount
				}

				StateMutex.Lock()

				// We use an explicit unlock at the end of the evaluation instead of a broken defer
				for _, svc := range Services {
					count := counts[svc]
					isCurrentlyOnline := count >= 30
					wasOnline := ServiceState[svc]

					if isCurrentlyOnline && !wasOnline {
						ServiceState[svc] = true
						logger.Log(logger.NewMessage(logger.INFO, logger.WATCHDOG, fmt.Sprintf("watchdog: %s is online, reported by [%s]", svc, config.Hostname)))

						publishHb(svc, count, true, Info, HeartbeatOnline)
						WatchdogQueue <- fmt.Sprintf("**RESOLVED:** Service **%s** is back online! Reported by [%s]", svc, config.Hostname)

					} else if !isCurrentlyOnline && wasOnline {
						ServiceState[svc] = false
						logger.Log(logger.NewMessage(logger.WARN, logger.WATCHDOG, fmt.Sprintf("watchdog: %s is offline, reported by [%s]", svc, config.Hostname)))

						publishHb(svc, count, false, Critical, HeartbeatFailed)
						WatchdogQueue <- fmt.Sprintf("**CRITICAL:** Service **%s** is down! (Heartbeats in last 60s: %v)", svc, count)
					}
				}
				StateMutex.Unlock()
			}
		}
	}
}

func AlertTeams(message string) {

	logger.Log(logger.NewMessage(logger.INFO, logger.WATCHDOG, "Alerting Teams"))

	if TeamsWebhook == "" {
		logger.Log(logger.NewMessage(logger.WARN, logger.WATCHDOG, "watchdog: Teams webhook URL is not configured"))
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
		logger.Log(logger.NewMessage(logger.ERROR, logger.WATCHDOG, fmt.Sprintf("watchdog: failed to marshal Teams alert payload: %v", err)))
		return
	}

	resp, err := http.Post(TeamsWebhook, "application/json", bytes.NewBuffer(body))
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.WATCHDOG, fmt.Sprintf("watchdog: failed to post Teams alert: %v", err)))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		buf := new(bytes.Buffer)
		buf.ReadFrom(resp.Body)
		logger.Log(logger.NewMessage(
			logger.ERROR,
			logger.WATCHDOG,
			fmt.Sprintf("watchdog: Teams webhook rejected the payload (status: %d, response: %s)", resp.StatusCode, buf.String())))
	}
}
