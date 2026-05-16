package checkin

import (
	"context"
	"fmt"
	"integration-project-ehb/controlroom/pkg/logger"
	"strings"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"
)

// usefull for mcp tools to get "opkomst statistieken"
func QueryAll(es *elasticsearch.Client, ctx context.Context) (*esapi.Response, error) {

	body := `
    {
        "size": 10000,
        "query": { "match_all": {} }
    }`

	res, err := es.Search(
		es.Search.WithContext(ctx),
		es.Search.WithIndex("checkin"),
		es.Search.WithBody(strings.NewReader(body)),
	)

	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM,
			fmt.Sprintf("query checkin failed for %v, here is the error: ", err)))
		return nil, err
	}

	return res, nil
}

// when did zico checkin into the festival, query checks the complete time range
func QueryCheckinTimeAllTime(
	es *elasticsearch.Client,
	ctx context.Context,
	uuid string) (*esapi.Response, error) {

	// size of one because we want one user
	body := fmt.Sprintf(`
    {
        "size": 1,
        "query": {
            "bool": {
                "must": [ { "term": { "id": "%s" } } ]
            }
        }
    }`,

		uuid)

	res, err := es.Search(
		es.Search.WithContext(ctx),
		es.Search.WithIndex("checkin"),
		es.Search.WithBody(strings.NewReader(body)),
	)

	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM,
			fmt.Sprintf("query user checkin failed for %s: %v", uuid, err)))
		return nil, err
	}

	return res, nil
}
