package heartbeat

import (
	"context"
	"encoding/xml"
	"fmt"
	"log"
	"time"

	"integration-project-ehb/controlroom/pkg/gen"

	"github.com/elastic/go-elasticsearch/v9"
)

// NOTE(nasr): returning a function independenant of es client to simplify mocking
func NewHeartbeatProcessor(es *elasticsearch.Client) func([]byte) error {
	return func(body []byte) error {
		var hb gen.HeartbeatType
		if err := xml.Unmarshal(body, &hb); err != nil {
			return fmt.Errorf("unmarshal: %w", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := indexHeartbeat(es, ctx, &hb); err != nil {
			return fmt.Errorf("index: %w", err)
		}

		log.Printf("Indexed Heartbeat: %s", hb.ServiceId)
		return nil
	}
}
