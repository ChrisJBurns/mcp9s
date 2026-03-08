package tui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/chrisjburns/mcp9s/internal/config"
	"github.com/chrisjburns/mcp9s/internal/mcpclient"
)

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
		statusMap := make(map[string]string)
		for _, s := range m.allServers {
			if s.Status != "" {
				statusMap[s.Name] = s.Status
			}
		}

		var probeCmds []tea.Cmd
		for i := range msg.servers {
			if status, ok := statusMap[msg.servers[i].Name]; ok {
				msg.servers[i].Status = status
			} else {
				probeCmds = append(probeCmds, probeServerCmd(msg.servers[i].Name, msg.servers[i].Server.URL))
			}
		}

		var cursorName string
		if m.cursor < len(m.filtered) {
			cursorName = m.filtered[m.cursor].Name
		}

		m.allServers = msg.servers
		m.clientCount = msg.clientCount
		m.warnings = msg.warnings
		m.applyFilter()

		if cursorName != "" {
			for i, s := range m.filtered {
				if s.Name == cursorName {
					m.cursor = i
					break
				}
			}
		}

		probeCmds = append(probeCmds, scheduleRefreshTick())
		return m, tea.Batch(probeCmds...)

	case serverStatusMsg:
		status := ""
		if msg.reachable {
			status = "Running"
		}
		for i := range m.allServers {
			if m.allServers[i].Name == msg.name {
				m.allServers[i].Status = status
			}
		}
		m.applyFilter()
		return m, nil

	case callToolMsg:
		m.responseLoading = false
		m.responseScroll = 0
		if msg.err != nil {
			m.responseText = "Error: " + msg.err.Error()
		} else {
			m.responseText = msg.response
		}
		m.responseLines = nil
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
		m.curlArgs = nil
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
			values := make([]string, paramCount)
			for i, f := range m.dialogFields {
				values[i] = f.Value()
			}
			args := mcpclient.BuildArgs(m.dialogTool.Params, values)
			serverURL := ""
			if m.cursor < len(m.filtered) {
				serverURL = mcpclient.StripFragment(m.filtered[m.cursor].Server.URL)
			}
			sessionID := m.detailSession.ID()
			m.requestText = mcpclient.BuildCurl(serverURL, sessionID, m.dialogTool.Name, args)
			ca := mcpclient.BuildCurlArgs(serverURL, sessionID, m.dialogTool.Name, args)
			m.curlArgs = &ca
			m.showToolDialog = false
			return m, nil
		}
		return m, m.dialogNext()

	case tea.KeyTab, tea.KeyDown:
		return m, m.dialogNext()

	case tea.KeyShiftTab, tea.KeyUp:
		return m, m.dialogPrev()
	}

	if !m.dialogOnOK && m.dialogParamIdx < paramCount {
		var cmd tea.Cmd
		m.dialogFields[m.dialogParamIdx], cmd = m.dialogFields[m.dialogParamIdx].Update(msg)
		return m, cmd
	}
	return m, nil
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
			m.detailServerNm = s.Name
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

func (m *model) dialogNext() tea.Cmd {
	paramCount := len(m.dialogTool.Params)
	if m.dialogOnOK {
		return nil
	}
	if m.dialogParamIdx < paramCount {
		m.dialogFields[m.dialogParamIdx].Blur()
	}
	if m.dialogParamIdx < paramCount-1 {
		m.dialogParamIdx++
		m.dialogFields[m.dialogParamIdx].Focus()
		return textinput.Blink
	}
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

func (m *model) applyFilter() {
	if m.filter == "" {
		m.filtered = m.allServers
	} else {
		re, err := regexp.Compile("(?i)" + m.filter)
		if err != nil {
			re = regexp.MustCompile(regexp.QuoteMeta(m.filter))
		}
		var out []config.ServerEntry
		for _, s := range m.allServers {
			clients := strings.Join(s.Clients, " ")
			if re.MatchString(s.Name) || re.MatchString(s.Server.URL) || re.MatchString(clients) {
				out = append(out, s)
			}
		}
		m.filtered = out
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}
