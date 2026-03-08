package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var clientImpl = &mcp.Implementation{
	Name:    "mcp9s",
	Version: "0.1.0",
}

// ToolParam describes a single input parameter for a tool.
type ToolParam struct {
	Name        string
	Type        string
	Description string
	Required    bool
}

// Tool is a simplified tool representation for display.
type Tool struct {
	Name        string // wire-protocol identifier used in tools/call
	Title       string // human-readable display name (may differ from Name)
	Description string
	Params      []ToolParam
}

// DisplayName returns the Title if set, otherwise the Name.
func (t Tool) DisplayName() string {
	if t.Title != "" {
		return t.Title
	}
	return t.Name
}

// Session wraps a live MCP client session.
type Session struct {
	session *mcp.ClientSession
}

// ID returns the session identifier.
func (s *Session) ID() string {
	if s == nil || s.session == nil {
		return ""
	}
	return s.session.ID()
}

// Close terminates the session.
func (s *Session) Close() {
	if s != nil && s.session != nil {
		s.session.Close()
	}
}

// FetchToolsResult holds the tools and live session from a successful fetch.
type FetchToolsResult struct {
	Tools   []Tool
	Session *Session
}

// buildTransports returns the ordered list of transports to try for a server URL.
func buildTransports(serverURL string) []mcp.Transport {
	return []mcp.Transport{
		&mcp.StreamableClientTransport{
			Endpoint:             serverURL,
			DisableStandaloneSSE: true,
			MaxRetries:           -1,
		},
		&mcp.SSEClientTransport{Endpoint: serverURL},
	}
}

// withSession connects to one of the transports and calls fn with the session.
// It tries each transport in order and returns the first success.
func withSession(serverURL string, timeout time.Duration, fn func(ctx context.Context, session *mcp.ClientSession) error) error {
	cleanURL := StripFragment(serverURL)
	transports := buildTransports(cleanURL)

	var errs []error
	for _, transport := range transports {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		client := mcp.NewClient(clientImpl, nil)
		session, err := client.Connect(ctx, transport, nil)
		if err != nil {
			cancel()
			errs = append(errs, err)
			continue
		}
		err = fn(ctx, session)
		cancel()
		if err == nil {
			return nil
		}
		session.Close()
		errs = append(errs, err)
	}
	if len(errs) == 0 {
		return fmt.Errorf("no transports available")
	}
	return errs[len(errs)-1]
}

// FetchTools connects to the MCP server at the given URL and returns its tools
// along with the live session. The caller must close the session when done.
func FetchTools(serverURL string) (*FetchToolsResult, error) {
	cleanURL := StripFragment(serverURL)
	transports := buildTransports(cleanURL)

	var errs []error
	for _, transport := range transports {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		client := mcp.NewClient(clientImpl, nil)
		session, err := client.Connect(ctx, transport, nil)
		if err != nil {
			cancel()
			errs = append(errs, err)
			continue
		}

		var tools []Tool
		for t, err := range session.Tools(ctx, nil) {
			if err != nil {
				session.Close()
				cancel()
				errs = append(errs, err)
				goto nextTransport
			}
			tools = append(tools, Tool{
				Name:        t.Name,
				Title:       t.Title,
				Description: t.Description,
				Params:      extractParams(t.InputSchema),
			})
		}
		cancel()
		return &FetchToolsResult{Tools: tools, Session: &Session{session: session}}, nil
	nextTransport:
	}

	if len(errs) == 0 {
		return nil, fmt.Errorf("no transports available")
	}
	return nil, errs[len(errs)-1]
}

// CallTool connects to the MCP server and invokes a tool with the given arguments.
func CallTool(serverURL, toolName string, args map[string]any) (string, error) {
	var response string
	err := withSession(serverURL, 30*time.Second, func(ctx context.Context, session *mcp.ClientSession) error {
		defer session.Close()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      toolName,
			Arguments: args,
		})
		if err != nil {
			return err
		}

		var parts []string
		for _, c := range result.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				parts = append(parts, tc.Text)
			}
		}

		resp := strings.Join(parts, "\n")
		if result.IsError {
			return fmt.Errorf("%s", resp)
		}
		response = resp
		return nil
	})
	return response, err
}

// extractParams parses JSON Schema input schema into a list of parameters.
func extractParams(schema any) []ToolParam {
	m, ok := schema.(map[string]any)
	if !ok {
		return nil
	}

	props, ok := m["properties"].(map[string]any)
	if !ok {
		return nil
	}

	requiredSet := map[string]bool{}
	if req, ok := m["required"].([]any); ok {
		for _, r := range req {
			if s, ok := r.(string); ok {
				requiredSet[s] = true
			}
		}
	}

	var params []ToolParam
	for name, val := range props {
		p := ToolParam{Name: name, Required: requiredSet[name]}
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
		if params[i].Required != params[j].Required {
			return params[i].Required
		}
		return params[i].Name < params[j].Name
	})

	return params
}

// BuildArgs converts string inputs to typed values based on param types.
func BuildArgs(params []ToolParam, values []string) map[string]any {
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
			num := json.Number(val)
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

// CurlArgs holds the structured components of a curl request for safe execution.
type CurlArgs struct {
	URL       string
	Headers   []string
	Body      []byte
	SessionID string
}

// BuildCurlArgs returns structured curl arguments for safe execution via exec.Command.
func BuildCurlArgs(serverURL, sessionID, toolName string, args map[string]any) CurlArgs {
	params := map[string]any{
		"name": toolName,
	}
	if len(args) > 0 {
		params["arguments"] = args
	}

	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params":  params,
	})

	return CurlArgs{
		URL:       serverURL,
		Headers:   []string{"Content-Type: application/json", "Accept: application/json, text/event-stream"},
		Body:      body,
		SessionID: sessionID,
	}
}

// ExecArgs returns the argument list for exec.Command("curl", ...).
func (c CurlArgs) ExecArgs() []string {
	args := []string{"-s", "-X", "POST", c.URL}
	for _, h := range c.Headers {
		args = append(args, "-H", h)
	}
	if c.SessionID != "" {
		args = append(args, "-H", "Mcp-Session-Id: "+c.SessionID)
	}
	args = append(args, "-d", string(c.Body))
	return args
}

// BuildCurl generates a display-friendly curl command string (not for execution).
func BuildCurl(serverURL, sessionID, toolName string, args map[string]any) string {
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

// StripFragment removes the URL fragment (everything after #).
func StripFragment(url string) string {
	if i := strings.Index(url, "#"); i != -1 {
		return url[:i]
	}
	return url
}
