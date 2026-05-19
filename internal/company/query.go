package company

import (
	"context"
	"fmt"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"
	"integration-project-ehb/controlroom/pkg/logger"
)

func QueryTotalSignedUpCompanies(ctx context.Context, es *elasticsearch.Client) (*esapi.Response, error) {
	res, err := es.Count(
		es.Count.WithContext(ctx),
		es.Count.WithIndex("users"),
	)

	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM,
			fmt.Sprintf("QueryTotalSignedUpUsers failed for : %v", err)))
		return nil, err
	}
	return res, nil
}
