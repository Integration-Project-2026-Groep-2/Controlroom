package userack

import (
	"context"
	"encoding/xml"
	"fmt"
	"time"

	"integration-project-ehb/controlroom/pkg/gen"
	"integration-project-ehb/controlroom/pkg/logger"

	"github.com/elastic/go-elasticsearch/v9"
)

func ProcessControlroomAck(es *elasticsearch.Client, body []byte) error {
	var ack gen.UserAck

	if err := xml.Unmarshal(body, &ack); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("Unmarshal error user_ack xml: %v", err.Error())))
		return fmt.Errorf("unmarshal error user_ack: %v", err)
	}

	doc := UserAckDoc{
		ID:      string(ack.UserId),
		Indexed: time.Now(),
		Service: ack.Service,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := indexUserAck(es, ctx, &doc); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("Failed to index user ack: %s", err.Error())))
		return fmt.Errorf("failed to index user ack: %s", err.Error())
	}

	logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, fmt.Sprintf("Processed User Ack for service: %s, user: %s", ack.Service, ack.UserId)))
	return nil
}

func ProcessCRMAck(es *elasticsearch.Client, body []byte) error {
	var dto gen.UserDoc

	if err := xml.Unmarshal(body, &dto); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("Unmarshal error CRM user_DTO xml: %v", err.Error())))
		return fmt.Errorf("unmarshal error CRM DTO: %v", err)
	}

	doc := UserAckDoc{
		ID:      string(dto.Id),
		Indexed: time.Now(),
		Service: "CRM",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := indexUserAck(es, ctx, &doc); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("Failed to index CRM user ack: %s", err.Error())))
		return fmt.Errorf("failed to index CRM user ack: %s", err.Error())
	}

	logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, fmt.Sprintf("Processed CRM User DTO as Ack for user: %s", dto.Id)))
	return nil
}
