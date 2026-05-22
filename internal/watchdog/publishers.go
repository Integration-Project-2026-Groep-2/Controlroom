package watchdog

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	config "integration-project-ehb/controlroom/internal/cr_config"
	"integration-project-ehb/controlroom/pkg/gen"
	"integration-project-ehb/controlroom/pkg/logger"

	amqp "github.com/rabbitmq/amqp091-go"
)

// publishHb is a watchdog function that publishes the current Heartbeat change to rabbit mq so that (frontend/mcp-master) can receive them and do something with it
func publishHb(svc string, count float64, up bool, severity SeverityLevel, event EventType) {

	ch := WatchdogChan.Load()

	if ch == nil {
		logger.Log(logger.NewMessage(logger.WARN, logger.WATCHDOG, "watchdog: RabbitMQ channel is not initialized"))
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
		logger.Log(logger.NewMessage(logger.ERROR, logger.WATCHDOG, fmt.Sprintf("watchdog: failed to encode heartbeat event to XML: %v", err)))
		return
	}

	if err := enc.Flush(); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.WATCHDOG, fmt.Sprintf("watchdog: failed to flush heartbeat XML encoder: %v", err)))
		return
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
		logger.Log(logger.NewMessage(logger.ERROR, logger.WATCHDOG, fmt.Sprintf("watchdog: failed to publish heartbeat event: %v", err)))
	}

}
