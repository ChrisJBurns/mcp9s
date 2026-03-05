package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// toolParam describes a single input parameter for a tool.
type toolParam struct {
	Name        string
	Type        string
	Description string
	Required    bool
}

// mcpTool is a simplified tool representation for display.
type mcpTool struct {
	Name        string
	Description string
	Params      []toolParam
}

// mcpSession wraps a live MCP client session.
type mcpSession struct {
	session *mcp.ClientSession
}

func (s *mcpSession) ID() string {
	if s == nil || s.session == nil {
		return ""
	}
	return s.session.ID()
}

func (s *mcpSession) Close() {
	if s != nil && s.session != nil {
		s.session.Close()
	}
}

// fetchToolsResult holds the tools and live session from a successful fetch.
type fetchToolsResult struct {
	tools   []mcpTool
	session *mcpSession
}

// fetchTools connects to the MCP server at the given URL and returns its tools
// along with the session ID from the initialized connection.
func fetchTools(serverURL string) (*fetchToolsResult, error) {
	cleanURL := stripFragment(serverURL)

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
		result, err := tryFetchTools(transport)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// callTool connects to the MCP server and invokes a tool with the given arguments.
func callTool(serverURL, toolName string, args map[string]any) (string, error) {
	cleanURL := stripFragment(serverURL)

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
		result, err := tryCallTool(transport, toolName, args)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return "", lastErr
}

func tryFetchTools(transport mcp.Transport) (*fetchToolsResult, error) {
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

	result, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		session.Close()
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
			Params:      extractParams(t.InputSchema),
		})
	}

	// Session is kept open — caller must close it when done
	return &fetchToolsResult{tools: tools, session: &mcpSession{session: session}}, nil
}

func tryCallTool(transport mcp.Transport, toolName string, args map[string]any) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "mcp9s",
		Version: "0.1.0",
	}, nil)

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return "", err
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: args,
	})
	if err != nil {
		return "", err
	}

	var parts []string
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}

	response := strings.Join(parts, "\n")
	if result.IsError {
		return "", fmt.Errorf("%s", response)
	}
	return response, nil
}

// extractParams parses JSON Schema input schema into a list of parameters.
func extractParams(schema any) []toolParam {
	m, ok := schema.(map[string]any)
	if !ok {
		return nil
	}

	props, ok := m["properties"].(map[string]any)
	if !ok {
		return nil
	}

	// Build required set
	requiredSet := map[string]bool{}
	if req, ok := m["required"].([]any); ok {
		for _, r := range req {
			if s, ok := r.(string); ok {
				requiredSet[s] = true
			}
		}
	}

	var params []toolParam
	for name, val := range props {
		p := toolParam{Name: name, Required: requiredSet[name]}
		if propMap, ok := val.(map[string]any); ok {
			if t, ok := propMap["type"].(string); ok {
				p.Type = t
			}
			if d, ok := propMap["description"].(string); ok {
				p.Description = d
			}
		}
		params = append(params, p)
	}

	sort.Slice(params, func(i, j int) bool {
		// Required params first, then alphabetical
		if params[i].Required != params[j].Required {
			return params[i].Required
		}
		return params[i].Name < params[j].Name
	})

	return params
}

// buildArgs converts string inputs to typed values based on param types.
func buildArgs(params []toolParam, values []string) map[string]any {
	args := make(map[string]any)
	for i, p := range params {
		if i >= len(values) {
			break
		}
		val := values[i]
		if val == "" {
			continue
		}

		switch p.Type {
		case "number", "integer":
			var num json.Number
			num = json.Number(val)
			if n, err := num.Int64(); err == nil {
				args[p.Name] = n
			} else if f, err := num.Float64(); err == nil {
				args[p.Name] = f
			} else {
				args[p.Name] = val
			}
		case "boolean":
			args[p.Name] = val == "true" || val == "1" || val == "yes"
		default:
			args[p.Name] = val
		}
	}
	return args
}

// buildCurl generates a single curl command for an MCP tools/call request,
// using the session ID from the already-initialized connection.
func buildCurl(serverURL, sessionID, toolName string, args map[string]any) string {
	params := map[string]any{
		"name": toolName,
	}
	if len(args) > 0 {
		params["arguments"] = args
	}

	body, _ := json.MarshalIndent(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params":  params,
	}, "", "  ")

	var b strings.Builder
	b.WriteString(fmt.Sprintf("curl -s -X POST '%s'", serverURL))
	b.WriteString(" \\\n  -H 'Content-Type: application/json'")
	b.WriteString(" \\\n  -H 'Accept: application/json, text/event-stream'")
	if sessionID != "" {
		b.WriteString(fmt.Sprintf(" \\\n  -H 'Mcp-Session-Id: %s'", sessionID))
	}
	b.WriteString(fmt.Sprintf(" \\\n  -d '%s'", string(body)))

	return b.String()
}

func stripFragment(url string) string {
	if i := strings.Index(url, "#"); i != -1 {
		return url[:i]
	}
	return url
}
