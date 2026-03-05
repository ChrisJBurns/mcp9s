package main

import (
	"fmt"
	"regexp"
	"strings"

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
	return nil
}

// ─── Update ─────────────────────────────────────────

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if key.Matches(msg, keys.ForceQ) {
			return m, tea.Quit
		}
		if m.showHelp {
			m.showHelp = false
			return m, nil
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
			m.view = viewDetail
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

	if m.view == viewDetail {
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
	urlW := iw * 40 / 100
	clientsW := iw * 25 / 100
	statusW := iw - nameW - urlW - clientsW
	for _, w := range []*int{&nameW, &urlW, &clientsW, &statusW} {
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
		tableHeaderStyle.Render(padRight("CLIENTS", clientsW)) +
		tableHeaderStyle.Render(padRight("STATUS", statusW))

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

		row := style.Render(padRight(truncate(s.name, nameW), nameW)) +
			style.Render(padRight(truncate(s.server.URL, urlW), urlW)) +
			style.Render(padRight(clientsStr, clientsW)) +
			style.Render(padRight(truncate(s.status, statusW), statusW))
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
	s := m.filtered[m.cursor]
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

	// Top panel — full width
	topTitle := tableTitleStyle.Render("Tools") +
		lipgloss.NewStyle().Foreground(colorAqua).Render("[") +
		tableTitleCountStyle.Render(s.name) +
		lipgloss.NewStyle().Foreground(colorAqua).Render("]")
	topContent := m.padToHeight("", topH)
	topBox := m.renderBorderedBox(topContent, topTitle, iw)

	// Bottom panels — split width (account for 4 chars border per box + 1 gap)
	// Each box has 4 chars of border overhead (│ + space + space + │)
	gap := 1
	bottomTotalInner := iw - 4 - gap // subtract one box's border + gap
	leftInnerW := bottomTotalInner / 2
	rightInnerW := bottomTotalInner - leftInnerW

	leftTitle := tableTitleStyle.Render("Request")
	leftContent := m.padToHeight("", bottomH)
	leftBox := m.renderBorderedBox(leftContent, leftTitle, leftInnerW)

	rightTitle := tableTitleStyle.Render("Response")
	rightContent := m.padToHeight("", bottomH)
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
	if m.view == viewDetail && m.cursor < len(m.filtered) {
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
