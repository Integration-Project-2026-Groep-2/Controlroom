package k8retriever

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"integration-project-ehb/controlroom/pkg/logger"
	"io"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"
)

const (
	k8sIndexName = "kubernetes-pods"
	timeout      = 5 * time.Second
)

func indexK8Pod(ctx context.Context, es *elasticsearch.Client, pod PodInfo) error {
	logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("k8retriever: indexing pod %s/%s", pod.Namespace, pod.Name)))

	doc := map[string]any{
		"namespace":               pod.Namespace,
		"name":                    pod.Name,
		"uid":                     pod.UID,
		"labels":                  pod.Labels,
		"annotations":             pod.Annotations,
		"phase":                   pod.Phase,
		"node":                    pod.Node,
		"ip":                      pod.IP,
		"host_ip":                 pod.HostIP,
		"service_account":         pod.ServiceAccount,
		"start_time":              pod.StartTime,
		"qos_class":               pod.QOSClass,
		"pod_ips":                 pod.PodIPs,
		"node_selector":           pod.NodeSelector,
		"tolerations":             pod.Tolerations,
		"containers":              pod.Containers,
		"init_containers":         pod.InitContainers,
		"conditions":              pod.Conditions,
		"container_statuses":      pod.ContainerStatuses,
		"init_container_statuses": pod.InitContainerStatuses,
		"cpu_request":             pod.CPURequest,
		"cpu_limit":               pod.CPULimit,
		"mem_request":             pod.MemRequest,
		"mem_limit":               pod.MemLimit,
		"container_count":         pod.ContainerCount,
		// note(nsar): causing name errors
		// "pod":                     pod.Pod,
		"@timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	}

	data, err := json.Marshal(doc)
	if err != nil {
		logger.Log(logger.NewMessage(
			logger.ERROR,
			logger.CONTROLROOM,
			fmt.Sprintf("k8retriever: failed to marshal pod %s/%s: %v", pod.Namespace, pod.Name, err),
		))
		return fmt.Errorf("marshal: %w", err)
	}

	documentID := fmt.Sprintf("%s-%s", pod.Namespace, pod.Name)
	req := esapi.IndexRequest{
		Index:      k8sIndexName,
		DocumentID: documentID,
		Body:       bytes.NewReader(data),
		Refresh:    "true",
	}

	res, err := req.Do(ctx, es)
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("k8retriever: failed to index pod %s/%s: %v", pod.Namespace, pod.Name, err)))
		return fmt.Errorf("index request: %w", err)
	}

	defer func(body io.ReadCloser) {
		if err := body.Close(); err != nil {
			logger.Log(logger.NewMessage(
				logger.WARN,
				logger.CONTROLROOM,
				fmt.Sprintf("k8retriever: failed to close response body for pod %s/%s: %v", pod.Namespace, pod.Name, err),
			))
		}
	}(res.Body)

	if res.IsError() {
		logger.Log(logger.NewMessage(
			logger.ERROR,
			logger.CONTROLROOM,
			fmt.Sprintf("k8retriever: Elasticsearch error indexing pod %s/%s: %s", pod.Namespace, pod.Name, res.String()),
		))
		return fmt.Errorf("elasticsearch: %s", res.String())
	}

	logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("k8retriever: successfully indexed pod %s/%s", pod.Namespace, pod.Name)))

	return nil
}
