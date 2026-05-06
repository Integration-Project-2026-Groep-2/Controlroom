package userack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"integration-project-ehb/controlroom/pkg/logger"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"
)

// UserAckDoc is de geünificeerde JSON structuur voor in de data view
type UserAckDoc struct {
	ID      string    `json:"id"`
	Indexed time.Time `json:"indexed"`
	Service string    `json:"service"`
}

func indexUserAck(es *elasticsearch.Client, ctx context.Context, doc *UserAckDoc) error {
	logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("indexing user ack for user %s from %s", doc.ID, doc.Service)))

	if es == nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, "elasticsearch client is nil"))
		return fmt.Errorf("elasticsearch client is nil")
	}

	// Converteer onze geünificeerde struct naar JSON
	jsonData, err := json.Marshal(doc)
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("failed to marshal user ack for %s: %v", doc.Service, err)))
		return err
	}

	// Stuur naar de user_acks index in Elastic
	req := esapi.IndexRequest{
		Index:      "user_acks",
		DocumentID: fmt.Sprintf("%s-%s", doc.Service, doc.ID), // Uniek ID: "KASSA-1234abcd"
		Body:       bytes.NewReader(jsonData),
		Refresh:    "true",
	}

	res, err := req.Do(ctx, es)
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("failed to index user ack for %s: %v", doc.Service, err)))
		return err
	}
	defer func(Body io.ReadCloser) {
		if err := Body.Close(); err != nil {
			logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, fmt.Sprintf("failed to close body after indexing user ack %s: %v", doc.Service, err)))
		}
	}(res.Body)

	if res.IsError() {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("elasticsearch error indexing user ack for %s: %s", doc.Service, res.String())))
		return fmt.Errorf("elasticsearch error: %s", res.String())
	}

	logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, fmt.Sprintf("indexed user ack for user %s from %s", doc.ID, doc.Service)))
	return nil
}
