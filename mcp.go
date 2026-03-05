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
// It tries Streamable HTTP first, then falls back to SSE if that fails.
func fetchTools(serverURL string) ([]mcpTool, error) {
	// Strip URL fragment — it's not sent to the server
	cleanURL := serverURL
	if i := strings.Index(cleanURL, "#"); i != -1 {
		cleanURL = cleanURL[:i]
	}

	transports := []mcp.Transport{
		&mcp.StreamableClientTransport{
			Endpoint:             cleanURL,
			DisableStandaloneSSE: true,
			MaxRetries:           -1,
		},
		&mcp.SSEClientTransport{Endpoint: cleanURL},
	}

	var lastErr error
	for _, transport := range transports {
		tools, err := tryFetchTools(transport)
		if err == nil {
			return tools, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func tryFetchTools(transport mcp.Transport) ([]mcpTool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

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
