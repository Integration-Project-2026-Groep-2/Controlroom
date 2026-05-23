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

	"github.com/elastic/go-elasticsearch/v9"

	"context"

	config "integration-project-ehb/controlroom/internal/cr_config"

	amqp "github.com/rabbitmq/amqp091-go"
)

func Generate(ctx context.Context, el *elasticsearch.Client, ch *amqp.Channel) error {

	var buf bytes.Buffer
	var body gen.Summary

	resp, err := user.QueryTotalSignedUpUsers(ctx, el)
	if err != nil {
		return fmt.Errorf("summary: failed to query total amount of users: %v", err)
	}

	// TODO(nasr): figure out how to do this properly later
	// parsing the json response of the elatic thing
	var countResult struct {
		Count int64 `json:"count"`
	}

	// Decode the JSON body into our struct
	if err := json.NewDecoder(resp.Body).Decode(&countResult); err != nil {
		return fmt.Errorf("summary: failed to decode user count response: %v", err)
	}

	// same thing
	resp, err = company.QueryTotalSignedUpCompanies(ctx, el)
	if err != nil {
		return fmt.Errorf("summary: failed to query total amount of companies: %v", err)
	}

	// Decode the JSON body into our struct
	if err := json.NewDecoder(resp.Body).Decode(&countResult); err != nil {
		return fmt.Errorf("summary: failed to decode company count response: %v", err)
	}

	enc := xml.NewEncoder(&buf)
	if err := enc.Encode(body); err != nil {
		return fmt.Errorf("summary: failed to encode summary XML: %v", err)
	}

	if err := enc.Flush(); err != nil {
		return fmt.Errorf("summary: failed to flush summary XML encoder: %v", err)

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
			return fmt.Errorf("summary: failed to publish heartbeat event: %v", err)

		}

	}

	return nil
}
