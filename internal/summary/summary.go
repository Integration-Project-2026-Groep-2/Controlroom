// TODO(nasr): think about this later
package summary

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"integration-project-ehb/controlroom/internal/company"
	"integration-project-ehb/controlroom/internal/user"
	"integration-project-ehb/controlroom/pkg/gen"
	"integration-project-ehb/controlroom/pkg/logger"

	"github.com/elastic/go-elasticsearch/v9"

	"context"

	config "integration-project-ehb/controlroom/internal/cr_config"

	amqp "github.com/rabbitmq/amqp091-go"
)

func Generate(ctx context.Context, el *elasticsearch.Client, ch *amqp.Channel) {

	var buf bytes.Buffer
	var body gen.Summary

	resp, err := user.QueryTotalSignedUpUsers(ctx, el)
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("summary: failed to query total amount of users: %v", err)))
		return

	}

	// TODO(nasr): figure out how to do this properly later
	// parsing the json response of the elatic thing
	var countResult struct {
		Count int64 `json:"count"`
	}

	// Decode the JSON body into our struct
	if err := json.NewDecoder(resp.Body).Decode(&countResult); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("summary: failed to decode user count response: %v", err)))
		return
	}

	// same thing
	resp, err = company.QueryTotalSignedUpCompanies(ctx, el)
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("summary: failed to query total amount of companies: %v", err)))
		return
	}

	// Decode the JSON body into our struct
	if err := json.NewDecoder(resp.Body).Decode(&countResult); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("summary: failed to decode company count response: %v", err)))
		return
	}

	enc := xml.NewEncoder(&buf)
	if err := enc.Encode(body); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("summary: failed to encode summary XML: %v", err)))
		return
	}

	if err := enc.Flush(); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("summary: failed to flush summary XML encoder: %v", err)))
		return
	}

	// publisher to mailing
	{
		if err := ch.PublishWithContext(
			context.Background(),
			config.Producer[config.SUMMARY_EVENT].Exchange.Name,
			config.Producer[config.SUMMARY_EVENT].Key.Key,
			false,
			false,
			amqp.Publishing{
				ContentType: "application/xml",
				Body:        buf.Bytes(),
			},
		); err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.WATCHDOG, fmt.Sprintf("summary: failed to publish heartbeat event: %v", err)))
		}

	}
}
