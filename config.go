package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tailscale/hujson"
	"gopkg.in/yaml.v3"
)

type MCPServer struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	URL     string            `json:"url"`
	Type    string            `json:"type"`
}

type serverEntry struct {
	name    string
	server  MCPServer
	status  string
	clients []string
}

// format describes how the config file is structured.
type format int

const (
	fmtJSON format = iota
	fmtJSONC
	fmtYAML
)

// clientDef describes where a specific MCP client stores its server config.
type clientDef struct {
	name       string
	path       string // relative to home dir
	serversKey string // JSON/YAML key holding the servers map
	format     format
	// For YAML configs with non-standard structure
	yamlParser func(data []byte) (map[string]MCPServer, error)
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

var clientDefs = []clientDef{
	// JSON clients with "mcpServers" key
	{name: "Claude Code", path: ".claude.json", serversKey: "mcpServers", format: fmtJSON},
	{name: "Cursor", path: ".cursor/mcp.json", serversKey: "mcpServers", format: fmtJSON},
	{name: "Windsurf", path: ".codeium/windsurf/mcp_config.json", serversKey: "mcpServers", format: fmtJSON},
	{name: "Windsurf JetBrains", path: ".codeium/mcp_config.json", serversKey: "mcpServers", format: fmtJSON},
	{name: "Kiro", path: ".kiro/settings/mcp.json", serversKey: "mcpServers", format: fmtJSON},
	{name: "LM Studio", path: ".lmstudio/mcp.json", serversKey: "mcpServers", format: fmtJSON},
	{name: "Gemini CLI", path: ".gemini/settings.json", serversKey: "mcpServers", format: fmtJSON},

	// JSONC clients with "servers" key (VSCode family)
	{name: "VSCode", path: "Library/Application Support/Code/User/mcp.json", serversKey: "servers", format: fmtJSONC},
	{name: "VSCode Insiders", path: "Library/Application Support/Code - Insiders/User/mcp.json", serversKey: "servers", format: fmtJSONC},

	// JSONC clients with "mcpServers" key (Roo Code, Cline as VSCode extensions store in settings)
	{name: "Roo Code", path: ".roo/mcp.json", serversKey: "mcpServers", format: fmtJSON},
	{name: "Cline", path: ".cline/mcp_settings.json", serversKey: "mcpServers", format: fmtJSON},
	{name: "Trae", path: ".trae/mcp.json", serversKey: "mcpServers", format: fmtJSON},
	{name: "Antigravity", path: ".antigravity/mcp.json", serversKey: "mcpServers", format: fmtJSON},

	// JSON with other keys
	{name: "OpenCode", path: ".opencode/mcp.json", serversKey: "mcp", format: fmtJSON},
	{name: "Zed", path: ".config/zed/settings.json", serversKey: "context_servers", format: fmtJSONC},

	// Amp clients (dotted key: "amp.mcpServers")
	{name: "Amp CLI", path: ".amp/settings.json", serversKey: "amp.mcpServers", format: fmtJSON},

	// YAML clients
	{name: "Goose", path: ".config/goose/config.yaml", serversKey: "extensions", format: fmtYAML, yamlParser: parseGooseConfig},
	{name: "Continue", path: ".continue/config.yaml", serversKey: "mcpServers", format: fmtYAML, yamlParser: parseContinueConfig},
}

// DiscoverServers scans all known MCP client configs and returns deduplicated server entries.
func DiscoverServers() ([]serverEntry, int) {
	home := homeDir()
	if home == "" {
		return nil, 0
	}

	seen := map[string]int{} // server name -> index in result
	var servers []serverEntry
	clientCount := 0

	for _, cd := range clientDefs {
		fullPath := filepath.Join(home, cd.path)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		parsed, err := parseClientConfig(cd, data)
		if err != nil {
			continue
		}

		if len(parsed) == 0 {
			continue
		}

		clientCount++

		for name, srv := range parsed {
			// Only include remote (SSE/Streamable HTTP) servers
			if srv.URL == "" {
				continue
			}
			if idx, ok := seen[name]; ok {
				servers[idx].clients = append(servers[idx].clients, cd.name)
			} else {
				seen[name] = len(servers)
				servers = append(servers, serverEntry{
					name:    name,
					server:  srv,
					status:  "",
					clients: []string{cd.name},
				})
			}
		}
	}

	sort.Slice(servers, func(i, j int) bool {
		return servers[i].name < servers[j].name
	})

	return servers, clientCount
}

// parseClientConfig dispatches to the right parser based on format.
func parseClientConfig(cd clientDef, data []byte) (map[string]MCPServer, error) {
	switch cd.format {
	case fmtYAML:
		if cd.yamlParser != nil {
			return cd.yamlParser(data)
		}
		return parseYAMLServers(data, cd.serversKey)
	case fmtJSONC:
		standardized, err := hujson.Standardize(data)
		if err != nil {
			return nil, err
		}
		return parseJSONServers(standardized, cd.serversKey)
	default:
		return parseJSONServers(data, cd.serversKey)
	}
}

// parseJSONServers extracts servers from a JSON blob using the given key.
// Supports dotted keys like "amp.mcpServers".
func parseJSONServers(data []byte, key string) (map[string]MCPServer, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	// Handle dotted keys
	parts := strings.Split(key, ".")
	current := raw
	for i, part := range parts {
		val, ok := current[part]
		if !ok {
			return nil, fmt.Errorf("key %q not found", part)
		}
		if i < len(parts)-1 {
			var next map[string]json.RawMessage
			if err := json.Unmarshal(val, &next); err != nil {
				return nil, err
			}
			current = next
		} else {
			// Final key — parse as servers map
			return parseServersMap(val)
		}
	}

	return nil, fmt.Errorf("key %q not found", key)
}

// parseServersMap parses a JSON object where keys are server names and values are server configs.
func parseServersMap(data json.RawMessage) (map[string]MCPServer, error) {
	// First try parsing with flexible field names
	var rawServers map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawServers); err != nil {
		return nil, err
	}

	result := make(map[string]MCPServer, len(rawServers))
	for name, serverData := range rawServers {
		srv, err := normalizeServer(serverData)
		if err != nil {
			continue // skip malformed entries
		}
		result[name] = srv
	}

	return result, nil
}

// normalizeServer parses a server entry, normalizing various URL field names.
func normalizeServer(data json.RawMessage) (MCPServer, error) {
	// Parse into a flexible structure
	var raw struct {
		Command   string            `json:"command"`
		Args      []string          `json:"args"`
		Env       map[string]string `json:"env"`
		URL       string            `json:"url"`
		ServerURL string            `json:"serverUrl"`
		URI       string            `json:"uri"`
		HttpURL   string            `json:"httpUrl"`
		Type      string            `json:"type"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return MCPServer{}, err
	}

	srv := MCPServer{
		Command: raw.Command,
		Args:    raw.Args,
		Env:     raw.Env,
		Type:    raw.Type,
	}

	// Normalize URL fields
	switch {
	case raw.URL != "":
		srv.URL = raw.URL
	case raw.ServerURL != "":
		srv.URL = raw.ServerURL
	case raw.URI != "":
		srv.URL = raw.URI
	case raw.HttpURL != "":
		srv.URL = raw.HttpURL
	}

	return srv, nil
}

// parseYAMLServers handles simple YAML configs with a top-level servers key.
func parseYAMLServers(data []byte, key string) (map[string]MCPServer, error) {
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	serversRaw, ok := raw[key]
	if !ok {
		return nil, fmt.Errorf("key %q not found", key)
	}

	// Re-marshal and unmarshal to MCPServer map
	b, err := json.Marshal(serversRaw)
	if err != nil {
		return nil, err
	}

	return parseServersMap(b)
}

// parseGooseConfig handles Goose's config.yaml format:
//
//	extensions:
//	  server-name:
//	    type: stdio
//	    cmd: command
//	    args: [...]
//	    env: {...}
func parseGooseConfig(data []byte) (map[string]MCPServer, error) {
	var raw struct {
		Extensions map[string]struct {
			Type string            `yaml:"type"`
			Cmd  string            `yaml:"cmd"`
			Args []string          `yaml:"args"`
			Env  map[string]string `yaml:"env"`
			URI  string            `yaml:"uri"`
		} `yaml:"extensions"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	result := make(map[string]MCPServer, len(raw.Extensions))
	for name, ext := range raw.Extensions {
		srv := MCPServer{
			Command: ext.Cmd,
			Args:    ext.Args,
			Env:     ext.Env,
			Type:    ext.Type,
			URL:     ext.URI,
		}
		result[name] = srv
	}

	return result, nil
}

// parseContinueConfig handles Continue's config.yaml format:
//
//	mcpServers:
//	  - name: server-name
//	    command: cmd
//	    args: [...]
//	    env: {...}
func parseContinueConfig(data []byte) (map[string]MCPServer, error) {
	var raw struct {
		MCPServers []struct {
			Name    string            `yaml:"name"`
			Command string            `yaml:"command"`
			Args    []string          `yaml:"args"`
			Env     map[string]string `yaml:"env"`
			URL     string            `yaml:"url"`
			Type    string            `yaml:"type"`
		} `yaml:"mcpServers"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	result := make(map[string]MCPServer, len(raw.MCPServers))
	for _, s := range raw.MCPServers {
		if s.Name == "" {
			continue
		}
		result[s.Name] = MCPServer{
			Command: s.Command,
			Args:    s.Args,
			Env:     s.Env,
			URL:     s.URL,
			Type:    s.Type,
		}
	}

	return result, nil
}
