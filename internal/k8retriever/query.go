package k8retriever

import (
	"bytes"
	"context"
	"fmt"
	"integration-project-ehb/controlroom/pkg/logger"
	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"
)

func QuerySummaryKubernetes(ctx context.Context, es *elasticsearch.Client, buf *bytes.Buffer) (*esapi.Response, error) {
	logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, "querying for kubernetes"))

	res, err := es.Search(
		es.Search.WithContext(ctx),
		es.Search.WithIndex("kubernetes-pods"),
		es.Search.WithBody(buf),
		es.Search.WithTrackTotalHits(true),
	)
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM,
			fmt.Sprintf("QuerySummaryKubernetes failed: %v", err)))
		return nil, err
	}
	return res, nil
}
