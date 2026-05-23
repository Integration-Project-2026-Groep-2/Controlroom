package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"integration-project-ehb/controlroom/internal/cr_github"
	"integration-project-ehb/controlroom/pkg/logger"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var CR_MCP_VERSION string = "1.0.0"
var CR_MCP_NAME string = "controlroom-mcp"

type SearchRequest struct {
	Size  int                            `json:"size"`
	Sort  []map[string]map[string]string `json:"sort"`
	Query any                            `json:"query"`
}

type ResultResponse struct {
	Hits HitsWrapper `json:"hits"`
}

type HitsWrapper struct {
	Hits []Hit `json:"hits"`
}

type Hit struct {
	Source map[string]any `json:"_source"`
}

// NOTE(nasr): mcp package takes network request as type any so we map them to a string here.
// parseArguments extracts the arguments map from a CallToolRequest.
func parseArguments(req mcp.CallToolRequest) map[string]any {
	m, _ := req.Params.Arguments.(map[string]any)
	return m
}

// formatDocs renders a slice of ES source docs as a readable text block.
func formatDocs(docs []map[string]any) string {
	if len(docs) == 0 {
		return "No results found."
	}
	var sb strings.Builder
	for i, doc := range docs {
		raw, _ := json.MarshalIndent(doc, "  ", "  ")
		fmt.Fprintf(&sb, "[%d]\n  %s\n\n", i+1, string(raw))
	}
	return sb.String()
}

func elasticQuery(index string, query any, size int, client *elasticsearch.Client) ([]map[string]any, error) {
	body := SearchRequest{
		Size: size,
		Sort: []map[string]map[string]string{
			{"timestamp": {"order": "desc"}},
		},
		Query: query,
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	res, err := client.Search(
		client.Search.WithContext(context.Background()),
		client.Search.WithIndex(index),
		client.Search.WithBody(&buf),
		client.Search.WithTrackTotalHits(true),
	)
	if err != nil {
		return nil, fmt.Errorf("search request: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch error: %s", res.String())
	}

	var result ResultResponse
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	docs := make([]map[string]any, 0, len(result.Hits.Hits))
	for _, h := range result.Hits.Hits {
		docs = append(docs, h.Source)
	}
	return docs, nil
}

func luceneQuery(q string) map[string]any {
	return map[string]any{"query_string": map[string]any{"query": q}}
}

func BuildFetchLogsQuery(service, gte, lte string) map[string]any {
	return map[string]any{
		"bool": map[string]any{
			"filter": []any{
				map[string]any{"term": map[string]any{"service.keyword": service}},
				map[string]any{"range": map[string]any{"timestamp": map[string]any{"gte": gte, "lte": lte}}},
				map[string]any{"terms": map[string]any{"level.keyword": []string{"ERROR", "WARN"}}},
			},
		},
	}
}

// newGithubConfig builds a GithubConfig from environment variables.
func newGithubConfig() cr_github.GithubConfig {
	return cr_github.GithubConfig{
		HTTP:  &http.Client{},
		Token: os.Getenv("GITHUB_TOKEN"),
		Org:   os.Getenv("ORG"),
	}
}

// resolveRepo fetches the org's repos and returns the repo name for the given
// service key. The lookup is case-insensitive. Returns an error tool result
// when the service is unknown.
func resolveRepo(ctx context.Context, config *cr_github.GithubConfig, service string) (string, *mcp.CallToolResult, error) {
	serviceRepos, err := cr_github.FetchRepos(ctx, config)
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.MCP, "github FetchRepos failed"))
		return "", mcp.NewToolResultError(fmt.Sprintf("github config error: %v", err)), err
	}

	repo, ok := serviceRepos[strings.ToLower(strings.TrimSpace(service))]
	if !ok {
		msg := fmt.Sprintf("unknown service '%s'", service)
		return "", mcp.NewToolResultError(msg), fmt.Errorf("error %s", msg)
	}
	return repo, nil, nil
}

func buildServer(client *elasticsearch.Client) *server.MCPServer {
	s := server.NewMCPServer(CR_MCP_NAME, CR_MCP_VERSION, server.WithToolCapabilities(false), server.WithRecovery())

	errorTool := mcp.NewTool("error_analysis",
		mcp.WithDescription("Query error logs from Elasticsearch. Accepts a Lucene query string (e.g. 'level:error AND service:controlroom')."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Lucene query string to filter error logs"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Max number of results to return (default 20)"),
		),
	)
	s.AddTool(errorTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := parseArguments(req)
		limit := 20

		query, ok := args["query"].(string)
		if !ok || strings.TrimSpace(query) == "" {
			return mcp.NewToolResultError("'query' must be a non-empty string"), nil
		}
		if raw, ok := args["limit"].(float64); ok && raw > 0 {
			limit = int(raw)
		}

		docs, err := elasticQuery("controlroom-logs", luceneQuery(query), limit, client)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Elasticsearch error: %v", err)), nil
		}
		return mcp.NewToolResultText(formatDocs(docs)), nil
	})

	heartbeatTool := mcp.NewTool("heartbeat_status",
		mcp.WithDescription("Fetch recent heartbeat events indexed from the RabbitMQ heartbeat consumer."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("service",
			mcp.Description("Filter by service name (optional, leave empty for all)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Max number of results to return (default 10)"),
		),
	)
	s.AddTool(heartbeatTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := parseArguments(req)
		limit := 10

		service, _ := args["service"].(string)
		if raw, ok := args["limit"].(float64); ok && raw > 0 {
			limit = int(raw)
		}

		query := "*"
		if strings.TrimSpace(service) != "" {
			query = fmt.Sprintf("service:%s", service)
		}

		docs, err := elasticQuery("heartbeats", luceneQuery(query), limit, client)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Elasticsearch error: %v", err)), nil
		}
		return mcp.NewToolResultText(formatDocs(docs)), nil
	})

	statusTool := mcp.NewTool("statuscheck_summary",
		mcp.WithDescription("Summarise recent statuscheck events indexed from the RabbitMQ statuscheck consumer."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("status",
			mcp.Description("Filter by status value, e.g. 'ok', 'degraded', 'down' (optional)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Max number of results to return (default 15)"),
		),
	)
	s.AddTool(statusTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := parseArguments(req)
		limit := 15

		status, _ := args["status"].(string)
		if raw, ok := args["limit"].(float64); ok && raw > 0 {
			limit = int(raw)
		}

		query := "*"
		if strings.TrimSpace(status) != "" {
			query = fmt.Sprintf("status:%s", status)
		}

		docs, err := elasticQuery("statuscheck", luceneQuery(query), limit, client)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Elasticsearch error: %v", err)), nil
		}
		return mcp.NewToolResultText(formatDocs(docs)), nil
	})

	fetchLogsTool := mcp.NewTool("fetch_logs",
		mcp.WithDescription("Fetch ERROR/WARN log entries for a service in a time-window centered around a failure timestamp. Server builds a typed bool query against the controlroom-logs Elasticsearch index. Returns up to 50 entries sorted by timestamp desc."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("service",
			mcp.Required(),
			mcp.Description("Service name (matches service.keyword exact-match field, e.g. 'kassa', 'crm')"),
		),
		mcp.WithString("since",
			mcp.Required(),
			mcp.Description("RFC3339 timestamp of the failure point; window centers around this"),
		),
		mcp.WithNumber("window_seconds",
			mcp.Description("Total window width in seconds (default 360 = 5min before + 1min after since)"),
		),
	)
	s.AddTool(fetchLogsTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := parseArguments(req)

		service, _ := args["service"].(string)
		since, _ := args["since"].(string)
		if strings.TrimSpace(service) == "" || strings.TrimSpace(since) == "" {
			return mcp.NewToolResultError("'service' and 'since' must be non-empty"), nil
		}

		window := 360
		if raw, ok := args["window_seconds"].(float64); ok && raw > 0 {
			window = int(raw)
		}

		sinceTs, err := time.Parse(time.RFC3339, since)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("'since' must be RFC3339: %v", err)), nil
		}

		gte := sinceTs.Add(-time.Duration(window) * time.Second * 5 / 6).UTC().Format(time.RFC3339)
		lte := sinceTs.Add(time.Duration(window) * time.Second / 6).UTC().Format(time.RFC3339)

		docs, err := elasticQuery("controlroom-logs", BuildFetchLogsQuery(service, gte, lte), 50, client)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Elasticsearch error: %v", err)), nil
		}
		return mcp.NewToolResultText(formatDocs(docs)), nil
	})

	fetchDeploysTool := mcp.NewTool("fetch_recent_deploys",
		mcp.WithDescription("Fetch the N most recent CD-workflow runs for a service via the GitHub Actions Runs API. Returns head_sha, created_at, conclusion, workflow_name, html_url per run, sorted by created_at desc."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("service",
			mcp.Required(),
			mcp.Description("Service name (kassa, crm, controlroom, frontend, mailing, facturatie, planning, iot, mcp-master)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Max runs to return (default 5, max 30)"),
		),
	)
	s.AddTool(fetchDeploysTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := parseArguments(req)

		service, _ := args["service"].(string)
		if strings.TrimSpace(service) == "" {
			return mcp.NewToolResultError("'service' must be non-empty"), nil
		}

		limit := 5
		if raw, ok := args["limit"].(float64); ok && raw > 0 {
			limit = int(raw)
		}

		config := newGithubConfig()
		repo, errResult, err := resolveRepo(ctx, &config, service)
		if err != nil {
			return errResult, nil
		}

		runs, err := cr_github.FetchRecentRuns(ctx, &config, repo, limit)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("github query failed: %v", err)), nil
		}

		docs := make([]map[string]any, len(runs))
		for i, r := range runs {
			docs[i] = map[string]any{
				"revision":      r.HeadSHA,
				"deployed_at":   r.CreatedAt.UTC().Format(time.RFC3339),
				"conclusion":    r.Conclusion,
				"workflow_name": r.WorkflowName,
				"html_url":      r.HTMLURL,
			}
		}
		return mcp.NewToolResultText(formatDocs(docs)), nil
	})

	fetchCommitsTool := mcp.NewTool("fetch_recent_commits",
		mcp.WithDescription("Fetch the N most recent commits for a service repository from GitHub."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("service",
			mcp.Required(),
			mcp.Description("Service name (kassa, crm, controlroom, frontend, mailing, facturatie, planning, iot, mcp-master)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Max commits to return (default 10, max 100)"),
		),
	)
	s.AddTool(fetchCommitsTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := parseArguments(req)

		service, _ := args["service"].(string)
		if strings.TrimSpace(service) == "" {
			return mcp.NewToolResultError("'service' must be non-empty"), nil
		}

		limit := 10
		if raw, ok := args["limit"].(float64); ok && raw > 0 {
			limit = int(raw)
		}

		config := newGithubConfig()
		repo, errResult, err := resolveRepo(ctx, &config, service)
		if err != nil {
			return errResult, nil
		}

		commits, err := cr_github.FetchRecentCommits(ctx, &config, repo, limit)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("github query failed: %v", err)), nil
		}

		docs := make([]map[string]any, 0, len(commits))
		for _, c := range commits {
			entry := map[string]any{}

			if sha, ok := c["sha"].(string); ok {
				entry["sha"] = sha
			}
			if commit, ok := c["commit"].(map[string]any); ok {
				if msg, ok := commit["message"].(string); ok {
					// Trim to first line only — body can be huge.
					entry["message"] = strings.SplitN(msg, "\n", 2)[0]
				}
				if author, ok := commit["author"].(map[string]any); ok {
					entry["author"] = author["name"]
					entry["date"] = author["date"]
				}
			}
			docs = append(docs, entry)
		}

		return mcp.NewToolResultText(formatDocs(docs)), nil
	})

	// NOTE(nasr): fetch_blob lets the LLM retrieve file contents by SHA so it
	// can read source code without needing a full tree walk. Pass the owner and
	// repo explicitly because blob SHAs are repo-scoped and service→repo
	// mapping alone is ambiguous when cross-repo reads are needed.
	fetchBlobTool := mcp.NewTool("fetch_blob",
		mcp.WithDescription("Fetch one or more files from GitHub by their Git blob SHA. Returns content and encoding (base64 or utf-8) per blob."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("owner",
			mcp.Required(),
			mcp.Description("GitHub repository owner (user or organisation)"),
		),
		mcp.WithString("repo",
			mcp.Required(),
			mcp.Description("Repository name"),
		),
		mcp.WithString("shas",
			mcp.Required(),
			mcp.Description("Comma-separated list of blob SHAs to fetch"),
		),
	)
	s.AddTool(fetchBlobTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := parseArguments(req)

		owner, _ := args["owner"].(string)
		repo, _ := args["repo"].(string)
		shaRaw, _ := args["shas"].(string)

		if strings.TrimSpace(owner) == "" || strings.TrimSpace(repo) == "" {
			return mcp.NewToolResultError("'owner' and 'repo' must be non-empty"), nil
		}
		if strings.TrimSpace(shaRaw) == "" {
			return mcp.NewToolResultError("'shas' must be a non-empty comma-separated list"), nil
		}

		shas := make([]string, 0)
		for s := range strings.SplitSeq(shaRaw, ",") {
			if trimmed := strings.TrimSpace(s); trimmed != "" {
				shas = append(shas, trimmed)
			}
		}
		if len(shas) == 0 {
			return mcp.NewToolResultError("no valid SHAs found in 'shas'"), nil
		}

		config := newGithubConfig()
		blobs, err := cr_github.GetBlob(ctx, &config, owner, repo, shas...)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("github blob fetch failed: %v", err)), nil
		}

		docs := make([]map[string]any, len(blobs))
		for i, b := range blobs {
			docs[i] = map[string]any{
				"sha":      b.FileSHA,
				"encoding": b.Encoding,
				"content":  b.Content,
			}
		}
		return mcp.NewToolResultText(formatDocs(docs)), nil
	})

	requestChangesTool := mcp.NewTool("request_changes",
		mcp.WithDescription("make a pull request"),
		mcp.WithReadOnlyHintAnnotation(false), // true for read-only; this mutates
		mcp.WithString("owner",
			mcp.Required(),
			mcp.Description("the owner of the repository"),
		),
		mcp.WithString("repo",
			mcp.Required(),
			mcp.Description("the repository name"),
		),
		mcp.WithString("title",
			mcp.Required(),
			mcp.Description("the title of the pull request"),
		),
		mcp.WithString("body",
			mcp.Required(),
			mcp.Description("the body of the pull request containing the information about the PR"),
		),
		mcp.WithString("head",
			mcp.Required(),
			mcp.Description("the head branch (e.g., 'feature-branch')"),
		),
		mcp.WithString("base",
			mcp.Required(),
			mcp.Description("the base branch (e.g., 'main')"),
		),
	)

	s.AddTool(requestChangesTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := parseArguments(req)
		owner, _ := args["owner"].(string)
		repo, _ := args["repo"].(string)
		title, _ := args["title"].(string)
		body, _ := args["body"].(string)
		head, _ := args["head"].(string)
		base, _ := args["base"].(string)

		// error handling
		{
			if strings.TrimSpace(owner) == "" {
				return mcp.NewToolResultError("'owner' must be non-empty"), nil
			}
			if strings.TrimSpace(repo) == "" {
				return mcp.NewToolResultError("'repo' must be non-empty"), nil
			}
			if strings.TrimSpace(title) == "" {
				return mcp.NewToolResultError("'title' must be non-empty"), nil
			}
			if strings.TrimSpace(body) == "" {
				return mcp.NewToolResultError("'body' must be non-empty"), nil
			}
			if strings.TrimSpace(head) == "" {
				return mcp.NewToolResultError("'head' must be non-empty"), nil
			}
			if strings.TrimSpace(base) == "" {
				return mcp.NewToolResultError("'base' must be non-empty"), nil
			}
		}

		config := newGithubConfig()
		result, err := cr_github.RequestChanges(ctx, &config, cr_github.PRResponse{
			Owner: owner,
			Repo:  repo,
			Title: title,
			Body:  body,
			Head:  head,
			Base:  base,
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to create pull request: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Pull request created successfully")), nil
	})

	return s
}

func SetupMCP(client *elasticsearch.Client) error {
	s := buildServer(client)

	port := os.Getenv("MCP_PORT")
	listenAddr := ":" + port

	httpServer := server.NewStreamableHTTPServer(s)

	logger.Log(logger.NewMessage(logger.INFO, logger.MCP, fmt.Sprintf("controlroom-mcp listening on %s (HTTP Stream)\n", listenAddr)))

	if err := httpServer.Start(listenAddr); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.MCP, fmt.Sprintf("mcp http stream server error: %v", err)))
		return fmt.Errorf("mcp http stream server: %w", err)
	}
	return nil
}
