package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

const DefaultBaseURL = "https://api.github.com"

type Run struct {
	HeadSHA      string    `json:"head_sha"`
	CreatedAt    time.Time `json:"created_at"`
	Conclusion   string    `json:"conclusion"`
	WorkflowName string    `json:"name"`
	HTMLURL      string    `json:"html_url"`
}

type Client struct {
	BaseURL string
	HTTP    *http.Client
	Token   string
}

func NewClient() *Client {
	return &Client{
		BaseURL: DefaultBaseURL,
		HTTP:    &http.Client{Timeout: 10 * time.Second},
		Token:   os.Getenv("GITHUB_TOKEN"),
	}
}

func (c *Client) FetchRecentRuns(ctx context.Context, repo string, limit int) ([]Run, error) {
	if limit <= 0 || limit > 30 {
		limit = 5
	}
	u := fmt.Sprintf("%s/repos/%s/actions/runs?head_branch=main&per_page=%d",
		c.BaseURL, repo, limit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("github actions runs API: %s", resp.Status)
	}

	var envelope struct {
		WorkflowRuns []Run `json:"workflow_runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, err
	}
	return envelope.WorkflowRuns, nil
}

var ServiceRepo = map[string]string{
	"kassa":       "Integration-Project-2026-Groep-2/Kassa",
	"crm":         "Integration-Project-2026-Groep-2/CRM",
	"controlroom": "Integration-Project-2026-Groep-2/Controlroom",
	"frontend":    "Integration-Project-2026-Groep-2/Frontend",
	"mailing":     "Integration-Project-2026-Groep-2/Mailing",
	"facturatie":  "Integration-Project-2026-Groep-2/Facturatie",
	"planning":    "Integration-Project-2026-Groep-2/Planning",
	"iot":         "Integration-Project-2026-Groep-2/IoT",
	"mcp-master":  "Integration-Project-2026-Groep-2/mcp-master",
}
