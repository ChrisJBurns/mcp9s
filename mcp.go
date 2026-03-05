package main

import (
	"context"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpTool is a simplified tool representation for display.
type mcpTool struct {
	Name        string
	Description string
}

// fetchTools connects to the MCP server at the given URL and returns its tools.
func fetchTools(url string) ([]mcpTool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	transport := pickTransport(url)

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "mcp9s",
		Version: "0.1.0",
	}, nil)

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	result, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		return nil, err
	}

	var tools []mcpTool
	for _, t := range result.Tools {
		name := t.Name
		if t.Title != "" {
			name = t.Title
		}
		tools = append(tools, mcpTool{
			Name:        name,
			Description: t.Description,
		})
	}

	return tools, nil
}

// pickTransport returns the appropriate MCP transport for the given URL.
// URLs containing "/sse" use the SSE transport; all others use Streamable HTTP.
func pickTransport(url string) mcp.Transport {
	if strings.Contains(url, "/sse") {
		return &mcp.SSEClientTransport{Endpoint: url}
	}
	return &mcp.StreamableClientTransport{
		Endpoint:             url,
		DisableStandaloneSSE: true,
		MaxRetries:           -1,
	}
}
