package k8retriever

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type ContainerInfo struct {
	Name            string   `json:"name"`
	Image           string   `json:"image"`
	ImagePullPolicy string   `json:"image_pull_policy"`
	Command         []string `json:"command"`
	Args            []string `json:"args"`
	Ports           []int32  `json:"ports"`
}

type ContainerStatusInfo struct {
	Name         string `json:"name"`
	Image        string `json:"image"`
	ImageID      string `json:"image_id"`
	ContainerID  string `json:"container_id"`
	Ready        bool   `json:"ready"`
	RestartCount int32  `json:"restart_count"`
	State        string `json:"state"`
	LastState    string `json:"last_state"`
}

type PodConditionInfo struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	Reason             string `json:"reason"`
	Message            string `json:"message"`
	LastProbeTime      string `json:"last_probe_time"`
	LastTransitionTime string `json:"last_transition_time"`
}

type PodInfo struct {
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
	Pod                   v1.Pod                `json:"pod"`
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

func formatTime(t *metav1.Time) string {
	if t == nil {
		return ""
	}

	return t.UTC().Format(time.RFC3339Nano)
}

func stringifyToleration(t v1.Toleration) string {
	parts := []string{}
	if t.Key != "" {
		parts = append(parts, "key="+t.Key)
	}
	if t.Operator != "" {
		parts = append(parts, "operator="+string(t.Operator))
	}
	if t.Value != "" {
		parts = append(parts, "value="+t.Value)
	}
	if t.Effect != "" {
		parts = append(parts, "effect="+string(t.Effect))
	}
	if t.TolerationSeconds != nil {
		parts = append(parts, fmt.Sprintf("seconds=%d", *t.TolerationSeconds))
	}

	if len(parts) == 0 {
		return "toleration"
	}

	return strings.Join(parts, ",")
}

func stringifyState(state v1.ContainerState) string {
	switch {
	case state.Running != nil:
		return "running"
	case state.Waiting != nil:
		return "waiting:" + state.Waiting.Reason
	case state.Terminated != nil:
		return "terminated:" + state.Terminated.Reason
	default:
		return "unknown"
	}
}

func toContainerInfo(container v1.Container) ContainerInfo {
	ports := make([]int32, 0, len(container.Ports))
	for _, port := range container.Ports {
		ports = append(ports, port.ContainerPort)
	}

	return ContainerInfo{
		Name:            container.Name,
		Image:           container.Image,
		ImagePullPolicy: string(container.ImagePullPolicy),
		Command:         append([]string(nil), container.Command...),
		Args:            append([]string(nil), container.Args...),
		Ports:           ports,
	}
}

func toContainerStatusInfo(status v1.ContainerStatus) ContainerStatusInfo {
	return ContainerStatusInfo{
		Name:         status.Name,
		Image:        status.Image,
		ImageID:      status.ImageID,
		ContainerID:  status.ContainerID,
		Ready:        status.Ready,
		RestartCount: status.RestartCount,
		State:        stringifyState(status.State),
		LastState:    stringifyState(status.LastTerminationState),
	}
}

func toPodConditionInfo(condition v1.PodCondition) PodConditionInfo {
	return PodConditionInfo{
		Type:               string(condition.Type),
		Status:             string(condition.Status),
		Reason:             condition.Reason,
		Message:            condition.Message,
		LastProbeTime:      formatTime(&condition.LastProbeTime),
		LastTransitionTime: formatTime(&condition.LastTransitionTime),
	}
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
			Namespace:             pod.Namespace,
			Name:                  pod.Name,
			UID:                   string(pod.UID),
			Labels:                pod.Labels,
			Annotations:           pod.Annotations,
			Phase:                 string(pod.Status.Phase),
			Node:                  pod.Spec.NodeName,
			IP:                    pod.Status.PodIP,
			HostIP:                pod.Status.HostIP,
			ServiceAccount:        pod.Spec.ServiceAccountName,
			StartTime:             formatTime(pod.Status.StartTime),
			QOSClass:              string(pod.Status.QOSClass),
			NodeSelector:          pod.Spec.NodeSelector,
			Tolerations:           make([]string, 0, len(pod.Spec.Tolerations)),
			PodIPs:                make([]string, 0, len(pod.Status.PodIPs)),
			Containers:            make([]ContainerInfo, 0, len(pod.Spec.Containers)),
			InitContainers:        make([]ContainerInfo, 0, len(pod.Spec.InitContainers)),
			Conditions:            make([]PodConditionInfo, 0, len(pod.Status.Conditions)),
			ContainerStatuses:     make([]ContainerStatusInfo, 0, len(pod.Status.ContainerStatuses)),
			InitContainerStatuses: make([]ContainerStatusInfo, 0, len(pod.Status.InitContainerStatuses)),
			CPURequest:            fmt.Sprintf("%dm", totalCPUReq),
			CPULimit:              fmt.Sprintf("%dm", totalCPULimit),
			MemRequest:            formatBytes(totalMemReq),
			MemLimit:              formatBytes(totalMemLimit),
			ContainerCount:        len(pod.Spec.Containers),
			Pod:                   pod,
		}

		for _, toleration := range pod.Spec.Tolerations {
			podInfo.Tolerations = append(podInfo.Tolerations, stringifyToleration(toleration))
		}

		for _, podIP := range pod.Status.PodIPs {
			podInfo.PodIPs = append(podInfo.PodIPs, podIP.IP)
		}

		for _, container := range pod.Spec.Containers {
			podInfo.Containers = append(podInfo.Containers, toContainerInfo(container))
		}

		for _, container := range pod.Spec.InitContainers {
			podInfo.InitContainers = append(podInfo.InitContainers, toContainerInfo(container))
		}

		for _, condition := range pod.Status.Conditions {
			podInfo.Conditions = append(podInfo.Conditions, toPodConditionInfo(condition))
		}

		for _, status := range pod.Status.ContainerStatuses {
			podInfo.ContainerStatuses = append(podInfo.ContainerStatuses, toContainerStatusInfo(status))
		}

		for _, status := range pod.Status.InitContainerStatuses {
			podInfo.InitContainerStatuses = append(podInfo.InitContainerStatuses, toContainerStatusInfo(status))
		}

		result = append(result, podInfo)
	}

	return result, nil
}
