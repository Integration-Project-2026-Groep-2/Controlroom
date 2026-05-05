package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

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

// NOTE(nasr): mcp package takes network request as type any so we map them to a stirng here
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

func elasticQuery(index, query string, size int, client *elasticsearch.Client) ([]map[string]any, error) {

	body := SearchRequest{
		Size: size,
		Sort: []map[string]map[string]string{
			{"timestamp": {"order": "desc"}},
		},
		Query: map[string]any{
			"query_string": map[string]any{
				"query": query,
			},
		},
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

	fmt.Println("the data:", docs)

	return docs, nil
}

func buildServer(client *elasticsearch.Client) *server.MCPServer {
	s := server.NewMCPServer(CR_MCP_NAME, CR_MCP_VERSION, server.WithToolCapabilities(false), server.WithRecovery())

	errorTool := mcp.NewTool("error_analysis",
		mcp.WithDescription("Query error logs from Elasticsearch. Accepts a Lucene query string (e.g. 'level:error AND service:controlroom')."),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("query string to filter error logs"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Max number of results to return (default 20)"),
		),
	)

	s.AddTool(errorTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {

		arguments := parseArguments(req)
		limit := 20

		query, ok := arguments["query"].(string)
		if !ok || strings.TrimSpace(query) == "" {
			return mcp.NewToolResultError("'query' must be a non-empty string"), nil
		}
		if raw, ok := arguments["limit"].(float64); ok && raw > 0 {
			limit = int(raw)
		}

		docs, err := elasticQuery("logs", query, limit, client)

		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Elasticsearch error: %v", err)), nil
		}
		return mcp.NewToolResultText(formatDocs(docs)), nil
	})

	heartbeatTool := mcp.NewTool("heartbeat_status",
		mcp.WithDescription("Fetch recent heartbeat events indexed from the RabbitMQ heartbeat consumer."),
		mcp.WithString("service",
			mcp.Description("Filter by service name (optional, leave empty for all)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Max number of results to return (default 10)"),
		),
	)
	s.AddTool(heartbeatTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {

		arguments := parseArguments(req)
		limit := 10

		service, _ := arguments["service"].(string)
		if raw, ok := arguments["limit"].(float64); ok && raw > 0 {
			limit = int(raw)
		}
		query := "*"
		if strings.TrimSpace(service) != "" {
			query = fmt.Sprintf("service: %s", service)
		}
		docs, err := elasticQuery("heartbeats", query, limit, client)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Elasticsearch error: %v", err)), nil
		}
		return mcp.NewToolResultText(formatDocs(docs)), nil
	})

	statusTool := mcp.NewTool("statuscheck_summary",
		mcp.WithDescription("Summarise recent statuscheck events indexed from the RabbitMQ statuscheck consumer."),
		mcp.WithString("status",
			mcp.Description("Filter by status value, e.g. 'ok', 'degraded', 'down' (optional)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Max number of results to return (default 15)"),
		),
	)

	s.AddTool(statusTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {

		arguments := parseArguments(req)
		limit := 15

		status, _ := arguments["status"].(string)
		if raw, ok := arguments["limit"].(float64); ok && raw > 0 {
			limit = int(raw)
		}
		query := "*"
		if strings.TrimSpace(status) != "" {
			query = fmt.Sprintf("status:%s", status)
		}
		docs, err := elasticQuery("statuscheck", query, limit, client)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Elasticsearch error: %v", err)), nil
		}
		return mcp.NewToolResultText(formatDocs(docs)), nil
	})

	return s
}

func SetupMCP(client *elasticsearch.Client) error {
	s := buildServer(client)

	port := os.Getenv("MCP_PORT")
	listenAddr := ":" + port

	log.Printf("Starting MCP server on %s", listenAddr)

	httpServer := server.NewStreamableHTTPServer(s)

	logger.Log(logger.NewMessage(logger.INFO, logger.MCP, fmt.Sprintf("controlroom-mcp listening on %s (HTTP Stream)\n", listenAddr)))

	if err := httpServer.Start(listenAddr); err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.MCP, fmt.Sprintf("mcp http stream server error: %v", err)))
		return fmt.Errorf("mcp http stream server: %w", err)
	}
	return nil
}
