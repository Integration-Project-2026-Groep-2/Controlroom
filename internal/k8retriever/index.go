package k8retriever

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"integration-project-ehb/controlroom/pkg/logger"
	"io"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"
)

const (
	k8sIndexName = "kubernetes-pods"
	timeout      = 5 * time.Second
)

type indexedK8Doc struct {
	Namespace             string                `json:"namespace"`
	Name                  string                `json:"name"`
	UID                   string                `json:"uid"`
	Labels                map[string]string     `json:"labels"`
	Annotations           map[string]string     `json:"annotations"`
	Phase                 string                `json:"phase"`
	Node                  string                `json:"node"`
	IP                    string                `json:"ip"`
	HostIP                string                `json:"host_ip"`
	ServiceAccount        string                `json:"service_account"`
	StartTime             string                `json:"start_time"`
	QOSClass              string                `json:"qos_class"`
	PodIPs                []string              `json:"pod_ips"`
	NodeSelector          map[string]string     `json:"node_selector"`
	Tolerations           []string              `json:"tolerations"`
	Containers            []ContainerInfo       `json:"containers"`
	InitContainers        []ContainerInfo       `json:"init_containers"`
	Conditions            []PodConditionInfo    `json:"conditions"`
	ContainerStatuses     []ContainerStatusInfo `json:"container_statuses"`
	InitContainerStatuses []ContainerStatusInfo `json:"init_container_statuses"`
	CPURequest            string                `json:"cpu_request"`
	CPULimit              string                `json:"cpu_limit"`
	MemRequest            string                `json:"mem_request"`
	MemLimit              string                `json:"mem_limit"`
	ContainerCount        int                   `json:"container_count"`
	Timestamp             string                `json:"@timestamp"`
}

func indexK8Pod(ctx context.Context, es *elasticsearch.Client, pod PodInfo) error {
	logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("k8retriever: indexing pod %s/%s", pod.Namespace, pod.Name)))

	doc := indexedK8Doc{
		Namespace:             pod.Namespace,
		Name:                  pod.Name,
		UID:                   pod.UID,
		Labels:                Sanitize(pod.Labels),
		Annotations:           filterAnnotations(Sanitize(pod.Annotations)),
		Phase:                 pod.Phase,
		Node:                  pod.Node,
		IP:                    pod.IP,
		HostIP:                pod.HostIP,
		ServiceAccount:        pod.ServiceAccount,
		StartTime:             pod.StartTime,
		QOSClass:              pod.QOSClass,
		PodIPs:                pod.PodIPs,
		NodeSelector:          Sanitize(pod.NodeSelector),
		Tolerations:           pod.Tolerations,
		Containers:            pod.Containers,
		InitContainers:        pod.InitContainers,
		Conditions:            pod.Conditions,
		ContainerStatuses:     pod.ContainerStatuses,
		InitContainerStatuses: pod.InitContainerStatuses,
		CPURequest:            pod.CPURequest,
		CPULimit:              pod.CPULimit,
		MemRequest:            pod.MemRequest,
		MemLimit:              pod.MemLimit,
		ContainerCount:        pod.ContainerCount,
		Timestamp:             time.Now().UTC().Format(time.RFC3339Nano),
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

// filterAnnotations removes known noisy annotation keys and truncates
// excessively long annotation values to avoid indexing huge payloads
// (e.g. kubectl.kubernetes.io/last-applied-configuration).
func filterAnnotations(annotations map[string]string) map[string]string {
	if len(annotations) == 0 {
		return nil
	}

	out := make(map[string]string, len(annotations))
	for k, v := range annotations {
		lk := strings.ToLower(k)
		if strings.Contains(lk, "last-applied") || strings.Contains(lk, "kubectl.kubernetes.io/last-applied-configuration") {
			// skip huge last-applied manifests
			continue
		}
		// truncate very long values
		if len(v) > 2048 {
			out[k] = v[:2048] + "...(truncated)"
		} else {
			out[k] = v
		}
	}

	if len(out) == 0 {
		return nil
	}
	return out
}
