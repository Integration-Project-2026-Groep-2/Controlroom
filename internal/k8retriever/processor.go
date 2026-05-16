package k8retriever

import (
	"context"
	"fmt"

	"github.com/elastic/go-elasticsearch/v9"
	"integration-project-ehb/controlroom/pkg/logger"
)

func ProcessK8sData(es *elasticsearch.Client) error {
	if es == nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, "elasticsearch client is nil"))
		return fmt.Errorf("elasticsearch client is nil")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	pods, err := GetPods(ctx)
	if err != nil {
		logger.Log(logger.NewMessage(
			logger.ERROR,
			logger.CONTROLROOM,
			fmt.Sprintf("failed to fetch kubernetes pods: %v", err),
		))
		return fmt.Errorf("fetch pods: %w", err)
	}

	if len(pods) == 0 {
		logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, "no kubernetes pods found"))
		return nil
	}

	// NOTE(nasr): important because we are dealing with an array
	indexedCount := 0
	for _, pod := range pods {
		if err := indexK8Pod(ctx, es, pod); err != nil {
			logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM, fmt.Sprintf("failed to index pod %s/%s: %v", pod.Namespace, pod.Name, err)))
			continue
		}
		indexedCount++
	}

	logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, fmt.Sprintf("indexed %d/%d kubernetes pods", indexedCount, len(pods))))

	return nil
}
