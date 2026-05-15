package heartbeat

import (
	"context"
	"integration-project-ehb/controlroom/pkg/logger"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
)

// HeartbeatQuery creates a query for recent heartbeats from a service
func HeartbeatQuery(service string, pageSize, withinSeconds int, es *elasticsearch.Client)  {

	res, err := es.Search().
	Index("index_name").
	Query(esdsl.NewMatchQuery("name", "Foo")).
	Size(pageSize)
	Do(context.Background())

	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, "failed to query for a heartbeat"))
	}

	// TODO(nasr): may 15 we we're here

}

