package k8retriever

import (
	"context"
	"fmt"
	"os"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type PodInfo struct {
	Namespace      string `json:"namespace"`
	Name           string `json:"name"`
	Phase          string `json:"phase"`
	Node           string `json:"node"`
	IP             string `json:"ip"`
	CPURequest     string `json:"cpu_request"`
	CPULimit       string `json:"cpu_limit"`
	MemRequest     string `json:"mem_request"`
	MemLimit       string `json:"mem_limit"`
	ContainerCount int    `json:"container_count"`
}

func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2fGi", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.2fMi", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.2fKi", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

func getKubeConfig() (*rest.Config, error) {
	return rest.InClusterConfig()
}

func GetPods(ctx context.Context) ([]PodInfo, error) {
	config, err := getKubeConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kube config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	const namespaceFile = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
	namespace := "default"
	if namespaceBytes, err := os.ReadFile(namespaceFile); err == nil {
		if resolvedNamespace := strings.TrimSpace(string(namespaceBytes)); resolvedNamespace != "" {
			namespace = resolvedNamespace
		}
	}

	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	var result []PodInfo

	for _, pod := range pods.Items {
		totalCPUReq := int64(0)
		totalCPULimit := int64(0)
		totalMemReq := int64(0)
		totalMemLimit := int64(0)

		// Aggregate resources across all containers
		for _, container := range pod.Spec.Containers {
			if cpu, ok := container.Resources.Requests[v1.ResourceCPU]; ok {
				totalCPUReq += cpu.MilliValue()
			}
			if cpu, ok := container.Resources.Limits[v1.ResourceCPU]; ok {
				totalCPULimit += cpu.MilliValue()
			}
			if mem, ok := container.Resources.Requests[v1.ResourceMemory]; ok {
				totalMemReq += mem.Value()
			}
			if mem, ok := container.Resources.Limits[v1.ResourceMemory]; ok {
				totalMemLimit += mem.Value()
			}
		}

		podInfo := PodInfo{
			Namespace:      pod.Namespace,
			Name:           pod.Name,
			Phase:          string(pod.Status.Phase),
			Node:           pod.Spec.NodeName,
			IP:             pod.Status.PodIP,
			CPURequest:     fmt.Sprintf("%dm", totalCPUReq),
			CPULimit:       fmt.Sprintf("%dm", totalCPULimit),
			MemRequest:     formatBytes(totalMemReq),
			MemLimit:       formatBytes(totalMemLimit),
			ContainerCount: len(pod.Spec.Containers),
		}

		result = append(result, podInfo)
	}

	return result, nil
}
