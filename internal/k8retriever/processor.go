package k8retriever

import (
	"context"
	"fmt"

	"integration-project-ehb/controlroom/pkg/logger"

	"github.com/elastic/go-elasticsearch/v9"
)

func ProcessK8sData(es *elasticsearch.Client) error {
	if es == nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, "k8retriever: Elasticsearch client is nil"))
		return fmt.Errorf("elasticsearch client is nil")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	pods, err := GetPods(ctx)
	if err != nil {
		logger.Log(logger.NewMessage(
			logger.ERROR,
			logger.CONTROLROOM,
			fmt.Sprintf("k8retriever: failed to fetch Kubernetes pods: %v", err),
		))
		return fmt.Errorf("fetch pods: %w", err)
	}

	if len(pods) == 0 {
		logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, "k8retriever: no Kubernetes pods found"))
		return nil
	}

	// NOTE(nasr): important because we are dealing with an array
	indexedCount := 0
	for _, pod := range pods {
		if err := indexK8Pod(ctx, es, pod); err != nil {
			logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, fmt.Sprintf("k8retriever: failed to index pod %s/%s: %v", pod.Namespace, pod.Name, err)))
			continue
		}
		indexedCount++
	}

	logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, fmt.Sprintf("k8retriever: indexed %d/%d Kubernetes pods", indexedCount, len(pods))))

	return nil
}
