package mcp

type K8Doclkdfjl struct {
	Namespace      string   `json:"namespace"`
	Name           string   `json:"name"`
	App            string   `json:"app"`
	Phase          string   `json:"phase"`
	Ready          bool     `json:"ready"`
	RestartCount   int      `json:"restart_count"`
	AgeSeconds     int64    `json:"age_seconds"`
	Node           string   `json:"node"`
	Image          string   `json:"image"`
	ContainerCount int      `json:"container_count"`
	CPURequest     string   `json:"cpu_request"`
	CPULimit       string   `json:"cpu_limit"`
	MemRequest     string   `json:"mem_request"`
	MemLimit       string   `json:"mem_limit"`
	Summary        string   `json:"summary"`
	Warnings       []string `json:"warnings"`
}
