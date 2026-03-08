package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ASCII logo — displayed in orange in the top-right, matching k9s style.
// Hidden when terminal is narrower than minLogoWidth.
const logo = ` ___  ___  ___ ___  ___  ___
|   \/   |/ __| _ \/ _ \/ __|
| |\/| | | (__|  _/ (_) \__ \
|_|  |_|\___|_|  \___/|___/`

const (
	minLogoWidth = 80 // hide logo below this terminal width
	minInfoWidth = 25 // info panel minimum
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
	tools      []mcpTool
	session    *mcpSession
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
	servers     []serverEntry
	clientCount int
}

type model struct {
	allServers  []serverEntry
	filtered    []serverEntry
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
	detailTools    []mcpTool
	detailLoading  bool
	detailError    string
	detailServerNm string
	detailSession  *mcpSession
	toolCursor     int

	// Tool call dialog
	showToolDialog bool
	dialogParamIdx int // index into dialogFields (params + OK/Cancel buttons)
	dialogFields   []textinput.Model
	dialogTool     *mcpTool
	dialogOnOK     bool // true when OK is focused

	// Request/Response panels
	requestText     string
	responseText    string
	responseLoading bool
	responseScroll  int
	responseLines   []string // pre-rendered (highlighted + wrapped) response lines
}

func newModel(servers []serverEntry, clientCount int) model {
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
		cmds = append(cmds, probeServerCmd(s.name, s.server.URL))
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
		servers, clientCount := DiscoverServers()
		return refreshResultMsg{servers: servers, clientCount: clientCount}
	}
}

// ─── Update ─────────────────────────────────────────

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case toolsMsg:
		if msg.serverName == m.detailServerNm {
			m.detailLoading = false
			if msg.err != nil {
				m.detailError = msg.err.Error()
			} else {
				m.detailTools = msg.tools
				m.detailSession = msg.session
			}
		}
		return m, nil

	case refreshTickMsg:
		return m, refreshServersCmd()

	case refreshResultMsg:
		// Build a map of existing statuses to preserve
		statusMap := make(map[string]string)
		for _, s := range m.allServers {
			if s.status != "" {
				statusMap[s.name] = s.status
			}
		}

		// Find new servers that need probing
		oldNames := make(map[string]bool)
		for _, s := range m.allServers {
			oldNames[s.name] = true
		}

		var probeCmds []tea.Cmd
		for i := range msg.servers {
			if status, ok := statusMap[msg.servers[i].name]; ok {
				msg.servers[i].status = status
			} else {
				// New server — probe it
				probeCmds = append(probeCmds, probeServerCmd(msg.servers[i].name, msg.servers[i].server.URL))
			}
		}

		// Preserve cursor position by name if possible
		var cursorName string
		if m.cursor < len(m.filtered) {
			cursorName = m.filtered[m.cursor].name
		}

		m.allServers = msg.servers
		m.clientCount = msg.clientCount
		m.applyFilter()

		// Restore cursor
		if cursorName != "" {
			for i, s := range m.filtered {
				if s.name == cursorName {
					m.cursor = i
					break
				}
			}
		}

		probeCmds = append(probeCmds, scheduleRefreshTick())
		return m, tea.Batch(probeCmds...)

	case serverStatusMsg:
		if msg.reachable {
			for i := range m.allServers {
				if m.allServers[i].name == msg.name {
					m.allServers[i].status = "Running"
				}
			}
			for i := range m.filtered {
				if m.filtered[i].name == msg.name {
					m.filtered[i].status = "Running"
				}
			}
		}
		return m, nil

	case callToolMsg:
		m.responseLoading = false
		m.responseScroll = 0
		if msg.err != nil {
			m.responseText = "Error: " + msg.err.Error()
		} else {
			m.responseText = msg.response
		}
		m.responseLines = nil // cleared; will be built on render
		return m, nil

	case tea.KeyMsg:
		if key.Matches(msg, keys.ForceQ) {
			return m, tea.Quit
		}
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		if m.showToolDialog {
			return m.updateToolDialog(msg)
		}
		if m.inputMode != inputNone {
			return m.updateInput(msg)
		}
		if m.view == viewDetail {
			return m.updateDetail(msg)
		}
		return m.updateServers(msg)
	}

	return m, nil
}

func (m model) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		val := m.textInput.Value()
		if m.inputMode == inputCommand {
			if val == "q" || val == "quit" {
				return m, tea.Quit
			}
		} else if m.inputMode == inputFilter {
			m.filter = val
			m.applyFilter()
		}
		m.inputMode = inputNone
		m.textInput.Blur()
		return m, nil

	case tea.KeyEsc:
		if m.inputMode == inputFilter {
			m.filter = ""
			m.applyFilter()
		}
		m.inputMode = inputNone
		m.textInput.Blur()
		return m, nil
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Back), key.Matches(msg, keys.Quit):
		m.view = viewServers
		m.toolCursor = 0
		m.responseText = ""
		m.responseLines = nil
		m.responseScroll = 0
		m.requestText = ""
		if m.detailSession != nil {
			m.detailSession.Close()
			m.detailSession = nil
		}
	case key.Matches(msg, keys.Up):
		if m.toolCursor > 0 {
			m.toolCursor--
		}
	case key.Matches(msg, keys.Down):
		if m.toolCursor < len(m.detailTools)-1 {
			m.toolCursor++
		}
	case key.Matches(msg, keys.Top):
		m.toolCursor = 0
	case key.Matches(msg, keys.Bottom):
		if len(m.detailTools) > 0 {
			m.toolCursor = len(m.detailTools) - 1
		}
	case key.Matches(msg, keys.Enter):
		if m.toolCursor < len(m.detailTools) {
			tool := &m.detailTools[m.toolCursor]
			m.dialogTool = tool
			m.dialogParamIdx = 0
			m.dialogOnOK = false
			m.showToolDialog = true

			// Create a textinput for each param
			m.dialogFields = make([]textinput.Model, len(tool.Params))
			for i, p := range tool.Params {
				ti := textinput.New()
				ti.Prompt = ""
				ti.Placeholder = p.Name
				ti.CharLimit = 256
				ti.Width = 40
				ti.TextStyle = detailValueStyle
				ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(colorLightSlateGray)
				ti.Cursor.Style = lipgloss.NewStyle().Foreground(colorAqua)
				if i == 0 {
					ti.Focus()
				}
				m.dialogFields[i] = ti
			}
			if len(tool.Params) == 0 {
				m.dialogOnOK = true
			}
			return m, textinput.Blink
		}
	case key.Matches(msg, keys.ScrollUp):
		if m.responseScroll > 0 {
			m.responseScroll--
		}
	case key.Matches(msg, keys.ScrollDown):
		if m.responseText != "" {
			m.responseScroll++
		}
	case key.Matches(msg, keys.Exec):
		if m.requestText != "" && !m.responseLoading {
			m.responseLoading = true
			m.responseText = ""
			m.responseLines = nil
			m.responseScroll = 0
			return m, m.execCurlCmd()
		}
	case key.Matches(msg, keys.Copy):
		if m.requestText != "" {
			copyToClipboard(m.requestText)
		}
	case key.Matches(msg, keys.Command):
		m.inputMode = inputCommand
		m.textInput.Prompt = "> "
		m.textInput.Placeholder = ""
		m.textInput.SetValue("")
		m.textInput.Focus()
		return m, textinput.Blink
	case key.Matches(msg, keys.Help):
		m.showHelp = true
	}
	return m, nil
}

func (m model) updateToolDialog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	paramCount := len(m.dialogTool.Params)

	switch msg.Type {
	case tea.KeyEsc:
		m.showToolDialog = false
		return m, nil

	case tea.KeyEnter:
		if m.dialogOnOK {
			// Build curl request and return to detail view
			values := make([]string, paramCount)
			for i, f := range m.dialogFields {
				values[i] = f.Value()
			}
			args := buildArgs(m.dialogTool.Params, values)
			serverURL := ""
			if m.cursor < len(m.filtered) {
				serverURL = stripFragment(m.filtered[m.cursor].server.URL)
			}
			m.requestText = buildCurl(serverURL, m.detailSession.ID(), m.dialogTool.Name, args)
			m.showToolDialog = false
			return m, nil
		}
		// Enter on a field — move to next field or OK
		return m, m.dialogNext()

	case tea.KeyTab, tea.KeyDown:
		return m, m.dialogNext()

	case tea.KeyShiftTab, tea.KeyUp:
		return m, m.dialogPrev()
	}

	// Pass key to the active text input
	if !m.dialogOnOK && m.dialogParamIdx < paramCount {
		var cmd tea.Cmd
		m.dialogFields[m.dialogParamIdx], cmd = m.dialogFields[m.dialogParamIdx].Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *model) dialogNext() tea.Cmd {
	paramCount := len(m.dialogTool.Params)
	if m.dialogOnOK {
		return nil
	}
	// Blur current field
	if m.dialogParamIdx < paramCount {
		m.dialogFields[m.dialogParamIdx].Blur()
	}
	if m.dialogParamIdx < paramCount-1 {
		m.dialogParamIdx++
		m.dialogFields[m.dialogParamIdx].Focus()
		return textinput.Blink
	}
	// Move to OK button
	m.dialogOnOK = true
	return nil
}

func (m *model) dialogPrev() tea.Cmd {
	if m.dialogOnOK {
		m.dialogOnOK = false
		paramCount := len(m.dialogTool.Params)
		if paramCount > 0 {
			m.dialogParamIdx = paramCount - 1
			m.dialogFields[m.dialogParamIdx].Focus()
			return textinput.Blink
		}
		return nil
	}
	if m.dialogParamIdx > 0 {
		m.dialogFields[m.dialogParamIdx].Blur()
		m.dialogParamIdx--
		m.dialogFields[m.dialogParamIdx].Focus()
		return textinput.Blink
	}
	return nil
}

func (m model) updateServers(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
	case key.Matches(msg, keys.Down):
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
	case key.Matches(msg, keys.Top):
		m.cursor = 0
	case key.Matches(msg, keys.Bottom):
		if len(m.filtered) > 0 {
			m.cursor = len(m.filtered) - 1
		}
	case key.Matches(msg, keys.Describe), key.Matches(msg, keys.Enter):
		if len(m.filtered) > 0 {
			s := m.filtered[m.cursor]
			if m.detailSession != nil {
				m.detailSession.Close()
				m.detailSession = nil
			}
			m.view = viewDetail
			m.detailTools = nil
			m.detailError = ""
			m.detailLoading = true
			m.detailServerNm = s.name
			m.requestText = ""
			m.responseText = ""
			return m, m.fetchToolsCmd(s)
		}
	case key.Matches(msg, keys.Filter):
		m.inputMode = inputFilter
		m.textInput.Prompt = "/ "
		m.textInput.Placeholder = "filter"
		m.textInput.SetValue(m.filter)
		m.textInput.Focus()
		return m, textinput.Blink
	case key.Matches(msg, keys.Command):
		m.inputMode = inputCommand
		m.textInput.Prompt = "> "
		m.textInput.Placeholder = ""
		m.textInput.SetValue("")
		m.textInput.Focus()
		return m, textinput.Blink
	case key.Matches(msg, keys.Help):
		m.showHelp = true
	}
	return m, nil
}

func (m model) fetchToolsCmd(s serverEntry) tea.Cmd {
	return func() tea.Msg {
		result, err := fetchTools(s.server.URL)
		if err != nil {
			return toolsMsg{serverName: s.name, err: err}
		}
		return toolsMsg{serverName: s.name, tools: result.tools, session: result.session}
	}
}

func (m model) callToolCmd(tool *mcpTool, args map[string]any) tea.Cmd {
	serverURL := ""
	if m.cursor < len(m.filtered) {
		serverURL = m.filtered[m.cursor].server.URL
	}
	toolName := tool.Name
	return func() tea.Msg {
		response, err := callTool(serverURL, toolName, args)
		return callToolMsg{response: response, err: err}
	}
}

// probeServerCmd checks if a server is reachable by attempting an MCP connection.
func probeServerCmd(name, serverURL string) tea.Cmd {
	return func() tea.Msg {
		_, err := fetchTools(serverURL)
		return serverStatusMsg{name: name, reachable: err == nil}
	}
}

// execCurlCmd runs the request curl command and returns the output as a callToolMsg.
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

func (m *model) applyFilter() {
	if m.filter == "" {
		m.filtered = m.allServers
	} else {
		re, err := regexp.Compile("(?i)" + m.filter)
		if err != nil {
			re = regexp.MustCompile(regexp.QuoteMeta(m.filter))
		}
		var out []serverEntry
		for _, s := range m.allServers {
			clients := strings.Join(s.clients, " ")
			if re.MatchString(s.name) || re.MatchString(s.server.URL) || re.MatchString(clients) {
				out = append(out, s)
			}
		}
		m.filtered = out
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

// ─── Layout helpers ─────────────────────────────────

// headerHeight returns the number of lines the header occupies.
func (m model) headerHeight() int {
	if m.showLogo() {
		return len(strings.Split(logo, "\n"))
	}
	// Compact header: just the info + menu rows
	return 3
}

func (m model) showLogo() bool {
	return m.width >= minLogoWidth
}

// innerWidth returns usable content width inside the bordered box.
// The border uses: │ (1) + space (1) + content + space (1) + │ (1) = 4 chars
func (m model) innerWidth() int {
	w := m.width - 4
	if w < 20 {
		w = 20
	}
	return w
}

// contentHeight returns the available lines for the table/detail content
// (excluding the border lines themselves).
func (m model) contentHeight() int {
	used := m.headerHeight() + 1 // +1 for crumbs
	if m.inputMode != inputNone {
		used += 3 // prompt border + content + border
	}
	used += 2 // table top + bottom border
	h := m.height - used
	if h < 3 {
		h = 3
	}
	return h
}

// ─── View ───────────────────────────────────────────

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	var sections []string

	sections = append(sections, m.renderHeader())

	if m.inputMode != inputNone {
		sections = append(sections, m.renderPrompt())
	}

	if m.showToolDialog {
		sections = append(sections, m.renderToolDialog())
	} else if m.view == viewDetail {
		sections = append(sections, m.renderDetail())
	} else {
		sections = append(sections, m.renderTable())
	}

	sections = append(sections, m.renderCrumbs())

	content := lipgloss.JoinVertical(lipgloss.Left, sections...)

	// Pad to full terminal height
	lines := strings.Split(content, "\n")
	for len(lines) < m.height {
		lines = append(lines, "")
	}
	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	result := strings.Join(lines, "\n")

	if m.showHelp {
		return m.overlayCenter(result, m.renderHelp())
	}

	return result
}

// ─── Header ─────────────────────────────────────────

func (m model) renderHeader() string {
	// Info panel (left)
	infoLines := []string{
		infoLabelStyle.Render("Clients: ") + infoValueStyle.Render(fmt.Sprintf("%d", m.clientCount)),
		infoLabelStyle.Render("Servers: ") + infoValueStyle.Render(fmt.Sprintf("%d", len(m.allServers))),
		infoLabelStyle.Render("Status:  ") + infoValueStyle.Render("Ready"),
	}

	// Menu / key hints (center)
	hints := hintBindings(m.view == viewDetail)
	menuLines := m.renderMenuGrid(hints)

	hHeight := m.headerHeight()

	if !m.showLogo() {
		// Narrow terminal: info left, menu right, no logo
		for len(infoLines) < hHeight {
			infoLines = append(infoLines, "")
		}
		for len(menuLines) < hHeight {
			menuLines = append(menuLines, "")
		}

		infoW := minInfoWidth
		menuW := m.width - infoW
		if menuW < 0 {
			menuW = 0
		}

		var rows []string
		for i := 0; i < hHeight; i++ {
			left := padRight(safeGet(infoLines, i), infoW)
			right := padRight(safeGet(menuLines, i), menuW)
			rows = append(rows, left+right)
		}
		return strings.Join(rows, "\n")
	}

	// Wide terminal: info | menu | logo
	logoLines := strings.Split(logo, "\n")
	logoW := 0
	for _, l := range logoLines {
		if len(l) > logoW {
			logoW = len(l)
		}
	}

	for len(infoLines) < hHeight {
		infoLines = append(infoLines, "")
	}
	for len(menuLines) < hHeight {
		menuLines = append(menuLines, "")
	}

	infoW := 30
	menuW := m.width - infoW - logoW - 2
	if menuW < 10 {
		menuW = 10
	}

	var rows []string
	for i := 0; i < hHeight; i++ {
		info := padRight(safeGet(infoLines, i), infoW)
		menu := padRight(safeGet(menuLines, i), menuW)
		lg := ""
		if i < len(logoLines) {
			lg = logoStyle.Render(logoLines[i])
		}
		rows = append(rows, info+menu+lg)
	}

	return strings.Join(rows, "\n")
}

func (m model) renderMenuGrid(bindings []key.Binding) []string {
	if len(bindings) == 0 {
		return nil
	}

	maxRows := 4
	if maxRows > len(bindings) {
		maxRows = len(bindings)
	}

	grid := make([][]string, maxRows)
	for i, b := range bindings {
		row := i % maxRows
		h := b.Help()
		cell := menuKeyStyle.Render("<"+h.Key+">") + " " + menuDescStyle.Render(h.Desc)
		grid[row] = append(grid[row], cell)
	}

	colWidth := 20
	var lines []string
	for _, row := range grid {
		var parts []string
		for _, cell := range row {
			parts = append(parts, padRight(cell, colWidth))
		}
		lines = append(lines, strings.Join(parts, " "))
	}

	// Trim trailing empty
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}

	return lines
}

// ─── Prompt ─────────────────────────────────────────

func (m model) renderPrompt() string {
	var bs lipgloss.Style
	if m.inputMode == inputCommand {
		bs = promptBorderCommandStyle
	} else {
		bs = promptBorderFilterStyle
	}

	w := m.innerWidth()
	return bs.Width(w).Render(m.textInput.View())
}

// ─── Table ──────────────────────────────────────────

func (m model) renderTable() string {
	iw := m.innerWidth()
	ch := m.contentHeight()

	// Column widths proportional to available space
	nameW := iw * 20 / 100
	urlW := iw * 35 / 100
	statusW := iw * 12 / 100
	clientsW := iw - nameW - urlW - statusW
	for _, w := range []*int{&nameW, &urlW, &statusW, &clientsW} {
		if *w < 6 {
			*w = 6
		}
	}

	// Table title
	title := tableTitleStyle.Render("Servers") +
		lipgloss.NewStyle().Foreground(colorAqua).Render("[") +
		tableTitleCountStyle.Render(fmt.Sprintf("%d", len(m.filtered))) +
		lipgloss.NewStyle().Foreground(colorAqua).Render("]")
	if m.filter != "" {
		title += " " + tableTitleFilterStyle.Render("</"+m.filter+">")
	}

	// Header row
	hdr := tableHeaderStyle.Render(padRight("NAME", nameW)) +
		tableHeaderStyle.Render(padRight("URL", urlW)) +
		tableHeaderStyle.Render(padRight("STATUS", statusW)) +
		tableHeaderStyle.Render(padRight("CLIENTS", clientsW))

	var lines []string
	lines = append(lines, hdr)

	// Scrolling window
	dataRows := ch - 1 // minus header
	if dataRows < 1 {
		dataRows = 1
	}
	start := 0
	if m.cursor >= dataRows {
		start = m.cursor - dataRows + 1
	}
	end := start + dataRows
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	for i := start; i < end; i++ {
		s := m.filtered[i]
		clientsStr := truncate(strings.Join(s.clients, ", "), clientsW)

		style := tableRowStyle
		if i == m.cursor {
			style = tableSelectedStyle
		}

		statusStyle := style
		if s.status == "Running" {
			statusStyle = lipgloss.NewStyle().Foreground(colorPaleGreen)
			if i == m.cursor {
				statusStyle = statusStyle.Background(colorAqua).Foreground(colorGreen)
			}
		}

		row := style.Render(padRight(truncate(s.name, nameW), nameW)) +
			style.Render(padRight(truncate(s.server.URL, urlW), urlW)) +
			statusStyle.Render(padRight(truncate(s.status, statusW), statusW)) +
			style.Render(padRight(clientsStr, clientsW))
		lines = append(lines, row)
	}

	for len(lines) < ch {
		lines = append(lines, strings.Repeat(" ", iw))
	}

	return m.renderBorderedBox(strings.Join(lines, "\n"), title, iw)
}

// ─── Server Detail View (3-panel layout) ────────────

func (m model) renderDetail() string {
	if m.cursor >= len(m.filtered) {
		return ""
	}
	ch := m.contentHeight()
	iw := m.innerWidth()

	// Split height: top panel gets ~40%, bottom panels share the rest
	topH := ch * 40 / 100
	if topH < 3 {
		topH = 3
	}
	// Bottom panels account for the border overhead of the top box (2 lines)
	bottomH := ch - topH - 2 // -2 for top box border lines
	if bottomH < 3 {
		bottomH = 3
	}

	// Top panel — full width: tools list
	toolCount := len(m.detailTools)
	topTitle := tableTitleStyle.Render("Tools") +
		lipgloss.NewStyle().Foreground(colorAqua).Render("[") +
		tableTitleCountStyle.Render(fmt.Sprintf("%d", toolCount)) +
		lipgloss.NewStyle().Foreground(colorAqua).Render("]")

	var topLines []string
	if m.detailLoading {
		topLines = append(topLines, detailValueStyle.Render("Loading tools..."))
	} else if m.detailError != "" {
		topLines = append(topLines, lipgloss.NewStyle().Foreground(colorOrangeRed).Render("Error: "+m.detailError))
	} else if toolCount == 0 {
		topLines = append(topLines, detailValueStyle.Render("No tools found"))
	} else {
		// Table header
		nameColW := iw * 30 / 100
		descColW := iw - nameColW
		topLines = append(topLines,
			tableHeaderStyle.Render(padRight("NAME", nameColW))+
				tableHeaderStyle.Render(padRight("DESCRIPTION", descColW)))

		// Scrolling window for tools
		dataRows := topH - 1 // minus header
		if dataRows < 1 {
			dataRows = 1
		}
		start := 0
		if m.toolCursor >= dataRows {
			start = m.toolCursor - dataRows + 1
		}
		end := start + dataRows
		if end > toolCount {
			end = toolCount
		}

		for i := start; i < end; i++ {
			t := m.detailTools[i]
			style := tableRowStyle
			if i == m.toolCursor {
				style = tableSelectedStyle
			}
			topLines = append(topLines,
				style.Render(padRight(truncate(t.Name, nameColW), nameColW))+
					style.Render(padRight(truncate(t.Description, descColW), descColW)))
		}
	}
	topContent := m.padToHeight(strings.Join(topLines, "\n"), topH)
	topBox := m.renderBorderedBox(topContent, topTitle, iw)

	// Bottom panels — split width (account for 4 chars border per box + 1 gap)
	// Each box has 4 chars of border overhead (│ + space + space + │)
	gap := 1
	bottomTotalInner := iw - 4 - gap // subtract one box's border + gap
	leftInnerW := bottomTotalInner / 2
	rightInnerW := bottomTotalInner - leftInnerW

	// Request panel — show curl command or selected tool's parameters
	leftTitle := tableTitleStyle.Render("Request")
	var leftLines []string
	if m.requestText != "" {
		for _, line := range strings.Split(m.requestText, "\n") {
			for len(line) > leftInnerW {
				leftLines = append(leftLines, detailValueStyle.Render(line[:leftInnerW]))
				line = line[leftInnerW:]
			}
			leftLines = append(leftLines, detailValueStyle.Render(line))
		}
	} else if m.toolCursor < len(m.detailTools) {
		tool := m.detailTools[m.toolCursor]
		if len(tool.Params) == 0 {
			leftLines = append(leftLines, detailValueStyle.Render("No parameters"))
		} else {
			for _, p := range tool.Params {
				req := ""
				if p.Required {
					req = lipgloss.NewStyle().Foreground(colorOrangeRed).Render("*")
				}
				line := detailKeyStyle.Render(p.Name) + req +
					detailColonStyle.Render(": ") +
					lipgloss.NewStyle().Foreground(colorLightSlateGray).Render(p.Type)
				leftLines = append(leftLines, line)
				if p.Description != "" {
					leftLines = append(leftLines, "  "+detailValueStyle.Render(truncate(p.Description, leftInnerW-2)))
				}
			}
		}
	}
	leftContent := m.padToHeight(strings.Join(leftLines, "\n"), bottomH)
	leftBox := m.renderBorderedBox(leftContent, leftTitle, leftInnerW)

	// Response panel
	rightTitle := tableTitleStyle.Render("Response")
	var rightLines []string
	if m.responseLoading {
		rightLines = append(rightLines, detailValueStyle.Render("Calling tool..."))
	} else if m.responseText != "" {
		m.responseLines = buildResponseLines(m.responseText, rightInnerW)
		// Clamp scroll
		maxScroll := len(m.responseLines) - bottomH
		if maxScroll < 0 {
			maxScroll = 0
		}
		if m.responseScroll > maxScroll {
			m.responseScroll = maxScroll
		}
		end := m.responseScroll + bottomH
		if end > len(m.responseLines) {
			end = len(m.responseLines)
		}
		rightLines = m.responseLines[m.responseScroll:end]
	}
	rightContent := m.padToHeight(strings.Join(rightLines, "\n"), bottomH)
	rightBox := m.renderBorderedBox(rightContent, rightTitle, rightInnerW)

	// Join left and right boxes side-by-side
	bottomRow := joinHorizontal(leftBox, rightBox, gap)

	return topBox + "\n" + bottomRow
}

// padToHeight returns content padded with empty lines to fill the given height.
func (m model) padToHeight(content string, height int) string {
	var lines []string
	if content != "" {
		lines = strings.Split(content, "\n")
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines[:height], "\n")
}

// joinHorizontal places two rendered boxes side by side with a gap between them.
func joinHorizontal(left, right string, gap int) string {
	leftLines := strings.Split(left, "\n")
	rightLines := strings.Split(right, "\n")

	maxLines := len(leftLines)
	if len(rightLines) > maxLines {
		maxLines = len(rightLines)
	}

	leftW := 0
	for _, l := range leftLines {
		if w := lipgloss.Width(l); w > leftW {
			leftW = w
		}
	}

	spacer := strings.Repeat(" ", gap)
	var out []string
	for i := 0; i < maxLines; i++ {
		l := ""
		if i < len(leftLines) {
			l = leftLines[i]
		}
		r := ""
		if i < len(rightLines) {
			r = rightLines[i]
		}
		out = append(out, padRight(l, leftW)+spacer+r)
	}
	return strings.Join(out, "\n")
}

// ─── Crumbs ─────────────────────────────────────────

func (m model) renderCrumbs() string {
	var crumbs string
	if m.showToolDialog && m.dialogTool != nil {
		crumbs = crumbStyle.Render("servers") + " " +
			crumbStyle.Render(m.detailServerNm) + " " +
			crumbActiveStyle.Render(m.dialogTool.Name)
	} else if m.view == viewDetail && m.cursor < len(m.filtered) {
		crumbs = crumbStyle.Render("servers") + " " +
			crumbActiveStyle.Render(m.filtered[m.cursor].name)
	} else {
		crumbs = crumbActiveStyle.Render("servers")
	}

	// Center the crumbs across the full width
	crumbW := lipgloss.Width(crumbs)
	leftPad := (m.width - crumbW) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	rightPad := m.width - crumbW - leftPad
	if rightPad < 0 {
		rightPad = 0
	}
	return strings.Repeat(" ", leftPad) + crumbs + strings.Repeat(" ", rightPad)
}

// ─── Tool Call Form (k9s-style) ─────────────────────

func (m model) renderToolDialog() string {
	tool := m.dialogTool
	iw := m.innerWidth()
	ch := m.contentHeight()

	dimStyle := lipgloss.NewStyle().Foreground(colorLightSlateGray)
	fieldBorder := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorDodgerBlue).
		Width(iw - 4)
	fieldBorderActive := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAqua).
		Width(iw - 4)

	title := tableTitleStyle.Render("Call Tool") +
		lipgloss.NewStyle().Foreground(colorAqua).Render("[") +
		tableTitleCountStyle.Render(tool.Name) +
		lipgloss.NewStyle().Foreground(colorAqua).Render("]")

	var lines []string

	if len(tool.Params) == 0 {
		lines = append(lines, "")
		lines = append(lines, detailValueStyle.Render("This tool has no parameters."))
		lines = append(lines, "")
	} else {
		for i, p := range tool.Params {
			// Label line: name (type) *
			label := detailKeyStyle.Render(p.Name)
			if p.Type != "" {
				label += " " + dimStyle.Render("("+p.Type+")")
			}
			if p.Required {
				label += lipgloss.NewStyle().Foreground(colorOrangeRed).Render(" *")
			}
			lines = append(lines, label)

			if p.Description != "" {
				lines = append(lines, dimStyle.Render(truncate(p.Description, iw)))
			}

			// Input field with border
			border := fieldBorder
			if i == m.dialogParamIdx && !m.dialogOnOK {
				border = fieldBorderActive
			}
			lines = append(lines, border.Render(m.dialogFields[i].View()))
			lines = append(lines, "")
		}
	}

	// OK / Cancel buttons
	okStyle := lipgloss.NewStyle().
		Foreground(colorBlack).
		Background(colorLightSlateGray).
		Padding(0, 3)
	cancelStyle := okStyle

	if m.dialogOnOK {
		okStyle = lipgloss.NewStyle().
			Foreground(colorBlack).
			Background(colorAqua).
			Bold(true).
			Padding(0, 3)
	}

	buttons := okStyle.Render("OK") + "  " + cancelStyle.Render("Cancel")
	btnW := lipgloss.Width(buttons)
	btnPad := (iw - btnW) / 2
	if btnPad < 0 {
		btnPad = 0
	}
	lines = append(lines, strings.Repeat(" ", btnPad)+buttons)

	// Hint line
	lines = append(lines, "")
	hint := dimStyle.Render("tab/↓ next • shift-tab/↑ prev • enter confirm • esc cancel")
	hintW := lipgloss.Width(hint)
	hintPad := (iw - hintW) / 2
	if hintPad < 0 {
		hintPad = 0
	}
	lines = append(lines, strings.Repeat(" ", hintPad)+hint)

	content := m.padToHeight(strings.Join(lines, "\n"), ch)
	return m.renderBorderedBox(content, title, iw)
}

// ─── Help Overlay ───────────────────────────────────

func (m model) renderHelp() string {
	title := tableTitleStyle.Render("Help")

	bindings := []struct{ key, desc string }{
		{"q", "Quit (not in input mode)"},
		{"↑/k", "Move up"},
		{"↓/j", "Move down"},
		{"g", "Go to top"},
		{"G", "Go to bottom"},
		{"d/enter", "Describe selected server"},
		{"/", "Activate filter (regex)"},
		{":", "Command mode (:q to quit)"},
		{"?", "Toggle help"},
		{"esc", "Back / cancel / clear filter"},
		{"ctrl+c", "Force quit"},
	}

	var lines []string
	lines = append(lines, title)
	lines = append(lines, "")
	for _, b := range bindings {
		lines = append(lines,
			menuKeyStyle.Render(padRight("<"+b.key+">", 14))+" "+menuDescStyle.Render(b.desc))
	}

	inner := strings.Join(lines, "\n")
	maxW := m.width - 6
	if maxW < 20 {
		maxW = 20
	}
	return promptBorderCommandStyle.Padding(1, 2).MaxWidth(maxW).Render(inner)
}

func (m model) overlayCenter(bg, overlay string) string {
	bgLines := strings.Split(bg, "\n")
	overlayLines := strings.Split(overlay, "\n")

	oHeight := len(overlayLines)
	oWidth := lipgloss.Width(overlay)

	startY := (m.height - oHeight) / 2
	startX := (m.width - oWidth) / 2
	if startY < 0 {
		startY = 0
	}
	if startX < 0 {
		startX = 0
	}

	for len(bgLines) < m.height {
		bgLines = append(bgLines, "")
	}

	for i, oLine := range overlayLines {
		y := startY + i
		if y >= len(bgLines) {
			break
		}
		bgLines[y] = padRight(bgLines[y], startX) + oLine
	}

	return strings.Join(bgLines, "\n")
}

// ─── Bordered Box ───────────────────────────────────

func (m model) renderBorderedBox(content, title string, innerWidth int) string {
	bc := lipgloss.NewStyle().Foreground(colorLightSkyBlue)

	tl, tr, bl, br := "╭", "╮", "╰", "╯"
	h, v := "─", "│"

	// The total width between corners = innerWidth + 2 (for the space padding on each side)
	borderFill := innerWidth + 2 // space inside │ content │

	// Top border: ╭──── title ────╮ (centered)
	titleStr := " " + title + " "
	titleVisualW := lipgloss.Width(titleStr)
	totalFill := borderFill - titleVisualW
	if totalFill < 0 {
		totalFill = 0
	}
	leftFill := totalFill / 2
	rightFill := totalFill - leftFill
	topBorder := bc.Render(tl+strings.Repeat(h, leftFill)) + titleStr + bc.Render(strings.Repeat(h, rightFill)+tr)

	// Bottom border: ╰────...──╯
	bottomBorder := bc.Render(bl + strings.Repeat(h, borderFill) + br)

	// Content lines with side borders
	contentLines := strings.Split(content, "\n")
	var result []string
	result = append(result, topBorder)
	for _, line := range contentLines {
		// Truncate lines that exceed inner width to prevent layout corruption
		if lipgloss.Width(line) > innerWidth {
			runes := []rune(line)
			for lipgloss.Width(string(runes)) > innerWidth && len(runes) > 0 {
				runes = runes[:len(runes)-1]
			}
			line = string(runes)
		}
		paddedLine := padRight(line, innerWidth)
		result = append(result, bc.Render(v)+" "+paddedLine+" "+bc.Render(v))
	}
	result = append(result, bottomBorder)

	return strings.Join(result, "\n")
}

// ─── Helpers ────────────────────────────────────────

func truncate(s string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	if len(s) <= maxW {
		return s
	}
	if maxW <= 3 {
		return s[:maxW]
	}
	return s[:maxW-3] + "..."
}

func padRight(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

func safeGet(sl []string, i int) string {
	if i < len(sl) {
		return sl[i]
	}
	return ""
}

func copyToClipboard(text string) {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(text)
	_ = cmd.Run()
}

// buildResponseLines pretty-prints JSON (if valid) with syntax highlighting,
// then word-wraps to fit the given width. Handles SSE "data: {...}" lines.
func buildResponseLines(text string, width int) []string {
	// Pre-process: extract JSON from SSE "data: " lines and join them
	display := extractAndFormatJSON(text)

	// Highlight then wrap using visual width
	var lines []string
	for _, raw := range strings.Split(display, "\n") {
		wrapped := wrapLine(raw, width)
		for _, chunk := range wrapped {
			lines = append(lines, highlightJSONLine(chunk))
		}
	}
	return lines
}

// wrapLine splits a string into chunks that fit within the given visual width.
func wrapLine(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	if lipgloss.Width(s) <= width {
		return []string{s}
	}
	var result []string
	runes := []rune(s)
	for len(runes) > 0 {
		// Binary-ish search: start from min(width, len) and shrink if needed
		cut := width
		if cut > len(runes) {
			cut = len(runes)
		}
		for cut > 1 && lipgloss.Width(string(runes[:cut])) > width {
			cut--
		}
		if cut == 0 {
			cut = 1
		}
		result = append(result, string(runes[:cut]))
		runes = runes[cut:]
	}
	if len(result) == 0 {
		result = []string{""}
	}
	return result
}

// extractAndFormatJSON tries to find and pretty-print JSON from the response.
// It handles raw JSON, SSE "data: {json}" lines, and mixed content.
func extractAndFormatJSON(text string) string {
	// First try: raw JSON
	var buf json.RawMessage
	if json.Unmarshal([]byte(text), &buf) == nil {
		if pretty, err := json.MarshalIndent(buf, "", "  "); err == nil {
			return string(pretty)
		}
	}

	// Second try: extract JSON from SSE "data: " lines
	var jsonParts []string
	var otherParts []string
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "data: ") {
			payload := strings.TrimPrefix(trimmed, "data: ")
			if json.Unmarshal([]byte(payload), &buf) == nil {
				if pretty, err := json.MarshalIndent(buf, "", "  "); err == nil {
					jsonParts = append(jsonParts, string(pretty))
					continue
				}
			}
			// data: line but not valid JSON
			otherParts = append(otherParts, line)
		} else if trimmed == "" {
			// skip empty SSE separator lines
			continue
		} else {
			otherParts = append(otherParts, line)
		}
	}

	if len(jsonParts) > 0 {
		all := append(otherParts, jsonParts...)
		return strings.Join(all, "\n")
	}

	return text
}

var jsonLineRe = regexp.MustCompile(`^(\s*)("(?:[^"\\]|\\.)*")\s*:\s*(.*)$`)

// highlightJSONLine applies k9s-themed colors to a single JSON line.
func highlightJSONLine(line string) string {
	keyStyle := lipgloss.NewStyle().Foreground(colorDodgerBlue).Bold(true)
	strStyle := lipgloss.NewStyle().Foreground(colorPaleGreen)
	numStyle := lipgloss.NewStyle().Foreground(colorFuchsia)
	boolStyle := lipgloss.NewStyle().Foreground(colorDarkOrange).Bold(true)
	nullStyle := lipgloss.NewStyle().Foreground(colorLightSlateGray)
	punctStyle := lipgloss.NewStyle().Foreground(colorLightSkyBlue)

	// Match "key": value lines
	if m := jsonLineRe.FindStringSubmatch(line); m != nil {
		indent := m[1]
		k := m[2]
		v := strings.TrimRight(m[3], " ")
		return indent + keyStyle.Render(k) + punctStyle.Render(": ") + colorJSONValue(v, strStyle, numStyle, boolStyle, nullStyle, punctStyle)
	}

	// Standalone values (array elements, closing braces, etc.)
	trimmed := strings.TrimSpace(line)
	indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	return indent + colorJSONValue(trimmed, strStyle, numStyle, boolStyle, nullStyle, punctStyle)
}

func colorJSONValue(v string, strStyle, numStyle, boolStyle, nullStyle, punctStyle lipgloss.Style) string {
	clean := strings.TrimRight(v, ",")
	trailing := v[len(clean):]

	switch {
	case strings.HasPrefix(clean, "\""):
		return strStyle.Render(clean) + punctStyle.Render(trailing)
	case clean == "true" || clean == "false":
		return boolStyle.Render(clean) + punctStyle.Render(trailing)
	case clean == "null":
		return nullStyle.Render(clean) + punctStyle.Render(trailing)
	case clean == "{" || clean == "}" || clean == "[" || clean == "]" ||
		clean == "{}" || clean == "[]":
		return punctStyle.Render(clean) + punctStyle.Render(trailing)
	case len(clean) > 0 && (clean[0] >= '0' && clean[0] <= '9' || clean[0] == '-'):
		return numStyle.Render(clean) + punctStyle.Render(trailing)
	default:
		return detailValueStyle.Render(v)
	}
}
