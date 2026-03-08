package tui

import (
	"fmt"
	"os/exec"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/chrisjburns/mcp9s/internal/config"
	"github.com/chrisjburns/mcp9s/internal/mcpclient"
)

const logo = ` ___  ___  ___ ___  ___  ___
|   \/   |/ __| _ \/ _ \/ __|
| |\/| | | (__|  _/ (_) \__ \
|_|  |_|\___|_|  \___/|___/`

const (
	minLogoWidth = 80
	minInfoWidth = 25
)

type viewState int

const (
	viewServers viewState = iota
	viewDetail
)

type inputMode int

const (
	inputNone inputMode = iota
	inputFilter
	inputCommand
)

// toolsMsg is sent when tools have been fetched from an MCP server.
type toolsMsg struct {
	serverName string
	tools      []mcpclient.Tool
	session    *mcpclient.Session
	err        error
}

// callToolMsg is sent when a tool call completes.
type callToolMsg struct {
	response string
	err      error
}

// serverStatusMsg reports whether a server is reachable.
type serverStatusMsg struct {
	name      string
	reachable bool
}

// refreshTickMsg triggers a re-scan of config files.
type refreshTickMsg struct{}

// refreshResultMsg carries the results of a config re-scan.
type refreshResultMsg struct {
	servers     []config.ServerEntry
	clientCount int
}

type model struct {
	allServers  []config.ServerEntry
	filtered    []config.ServerEntry
	clientCount int
	cursor      int
	width       int
	height      int

	view      viewState
	inputMode inputMode
	textInput textinput.Model
	filter    string

	showHelp bool

	// Detail view state
	detailTools    []mcpclient.Tool
	detailLoading  bool
	detailError    string
	detailServerNm string
	detailSession  *mcpclient.Session
	toolCursor     int

	// Tool call dialog
	showToolDialog bool
	dialogParamIdx int
	dialogFields   []textinput.Model
	dialogTool     *mcpclient.Tool
	dialogOnOK     bool

	// Request/Response panels
	requestText     string
	responseText    string
	responseLoading bool
	responseScroll  int
	responseLines   []string
}

// NewModel creates the initial TUI model.
func NewModel(servers []config.ServerEntry, clientCount int) model {
	ti := textinput.New()
	ti.CharLimit = 128
	ti.Prompt = ""
	ti.TextStyle = promptTextStyle
	ti.PromptStyle = promptTextStyle
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(colorAqua)

	return model{
		allServers:  servers,
		filtered:    servers,
		clientCount: clientCount,
		textInput:   ti,
	}
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{scheduleRefreshTick()}
	for _, s := range m.allServers {
		cmds = append(cmds, probeServerCmd(s.Name, s.Server.URL))
	}
	return tea.Batch(cmds...)
}

func scheduleRefreshTick() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return refreshTickMsg{}
	})
}

func refreshServersCmd() tea.Cmd {
	return func() tea.Msg {
		servers, clientCount := config.DiscoverServers()
		return refreshResultMsg{servers: servers, clientCount: clientCount}
	}
}

func (m model) fetchToolsCmd(s config.ServerEntry) tea.Cmd {
	return func() tea.Msg {
		result, err := mcpclient.FetchTools(s.Server.URL)
		if err != nil {
			return toolsMsg{serverName: s.Name, err: err}
		}
		return toolsMsg{serverName: s.Name, tools: result.Tools, session: result.Session}
	}
}

func (m model) callToolCmd(tool *mcpclient.Tool, args map[string]any) tea.Cmd {
	serverURL := ""
	if m.cursor < len(m.filtered) {
		serverURL = m.filtered[m.cursor].Server.URL
	}
	toolName := tool.Name
	return func() tea.Msg {
		response, err := mcpclient.CallTool(serverURL, toolName, args)
		return callToolMsg{response: response, err: err}
	}
}

func probeServerCmd(name, serverURL string) tea.Cmd {
	return func() tea.Msg {
		_, err := mcpclient.FetchTools(serverURL)
		return serverStatusMsg{name: name, reachable: err == nil}
	}
}

func (m model) execCurlCmd() tea.Cmd {
	curlText := m.requestText
	return func() tea.Msg {
		out, err := exec.Command("sh", "-c", curlText).CombinedOutput()
		if err != nil {
			return callToolMsg{err: fmt.Errorf("%s: %s", err, string(out))}
		}
		return callToolMsg{response: string(out)}
	}
}

// Layout helpers

func (m model) headerHeight() int {
	if m.showLogo() {
		return len(splitLines(logo))
	}
	return 3
}

func (m model) showLogo() bool {
	return m.width >= minLogoWidth
}

func (m model) innerWidth() int {
	w := m.width - 4
	if w < 20 {
		w = 20
	}
	return w
}

func (m model) contentHeight() int {
	used := m.headerHeight() + 1
	if m.inputMode != inputNone {
		used += 3
	}
	used += 2
	h := m.height - used
	if h < 3 {
		h = 3
	}
	return h
}

func (m model) padToHeight(content string, height int) string {
	var lines []string
	if content != "" {
		lines = splitLines(content)
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return joinLines(lines[:height])
}
