package warning_producer

import (
	"context"
	"fmt"
	"integration-project-ehb/controlroom/pkg/analyser"
	"integration-project-ehb/controlroom/pkg/logger"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	amqp "github.com/rabbitmq/amqp091-go"
)

func RunWarningProducer(es *elasticsearch.Client, ctx context.Context, ch *amqp.Channel) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		fmt.Println("Loop")
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			warnings, err := analyser.CheckWarnings(es)
			if err != nil {
				logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("failed to fetch data: %v", err)))
			}

			if len(warnings) > 0 {
				for _, value := range warnings {
					// Publish to the exchange
					err = ch.PublishWithContext(ctx,
						"news.topic",   // exchange
						"news.warning", // routing key (matches your setup() binding)
						false,
						false,
						amqp.Publishing{
							ContentType: "text/plain",
							Body:        []byte(value),
						})

					if err != nil {
						logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("failed to publish news: %v", err)))
					} else {
						logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, "published news warning"))
					}
				}
			}
		}
	}
}
