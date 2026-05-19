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

func Generate( ctx context.Context, el *elasticsearch.Client, ch *amqp.Channel) {

	var buf bytes.Buffer
	var body gen.Summary

	resp, err :=  user.QueryTotalSignedUpUsers(ctx, el)
	// PRANK
	if err != nil {
		logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, "failed to query total amount of users, HAHAHAH NOT GIVING YOU THE ERROR GET PRANKED"))

	}

	// TODO(nasr): figure out how to do this properly later
	// parsing the json response of the elatic thing
	var countResult struct {
		Count int64 `json:"count"`
	}

	// Decode the JSON body into our struct
	if err := json.NewDecoder(resp.Body).Decode(&countResult); err != nil {
		logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, fmt.Sprintf("failed to decode the json response from elastic", err)))
		return
	}

	// same thing
	resp, err = company.QueryTotalSignedUpCompanies(ctx, el)
	// you are getting errors :)
	if err != nil {
		logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, fmt.Sprintf("failed to query total amount of companies, error: %v", err)))
	}

	// Decode the JSON body into our struct
	if err := json.NewDecoder(resp.Body).Decode(&countResult); err != nil {
		logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, fmt.Sprintf("failed to decode the json response from elastic", err)))
		return
	}

	enc := xml.NewEncoder(&buf)
	if err := enc.Encode(body); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("failed to encode to xml: %v", err)))
		return
	}


	if err := enc.Flush(); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("failed to encode to xml, (flush thing): %v", err)))
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


