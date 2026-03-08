package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

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

	lines := splitLines(content)
	for len(lines) < m.height {
		lines = append(lines, "")
	}
	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	result := joinLines(lines)

	if m.showHelp {
		return m.overlayCenter(result, m.renderHelp())
	}

	return result
}

// ─── Header ─────────────────────────────────────────

func (m model) renderHeader() string {
	infoLines := []string{
		infoLabelStyle.Render("Clients: ") + infoValueStyle.Render(fmt.Sprintf("%d", m.clientCount)),
		infoLabelStyle.Render("Servers: ") + infoValueStyle.Render(fmt.Sprintf("%d", len(m.allServers))),
		infoLabelStyle.Render("Status:  ") + infoValueStyle.Render("Ready"),
	}

	hints := hintBindings(m.view == viewDetail)
	menuLines := m.renderMenuGrid(hints)

	hHeight := m.headerHeight()

	if !m.showLogo() {
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
		return joinLines(rows)
	}

	logoLines := splitLines(logo)
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

	return joinLines(rows)
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

	nameW := iw * 20 / 100
	urlW := iw * 35 / 100
	statusW := iw * 12 / 100
	clientsW := iw - nameW - urlW - statusW
	for _, w := range []*int{&nameW, &urlW, &statusW, &clientsW} {
		if *w < 6 {
			*w = 6
		}
	}

	title := tableTitleStyle.Render("Servers") +
		lipgloss.NewStyle().Foreground(colorAqua).Render("[") +
		tableTitleCountStyle.Render(fmt.Sprintf("%d", len(m.filtered))) +
		lipgloss.NewStyle().Foreground(colorAqua).Render("]")
	if m.filter != "" {
		title += " " + tableTitleFilterStyle.Render("</"+m.filter+">")
	}

	hdr := tableHeaderStyle.Render(padRight("NAME", nameW)) +
		tableHeaderStyle.Render(padRight("URL", urlW)) +
		tableHeaderStyle.Render(padRight("STATUS", statusW)) +
		tableHeaderStyle.Render(padRight("CLIENTS", clientsW))

	var lines []string
	lines = append(lines, hdr)

	dataRows := ch - 1
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
		clientsStr := truncate(strings.Join(s.Clients, ", "), clientsW)

		style := tableRowStyle
		if i == m.cursor {
			style = tableSelectedStyle
		}

		statusStyle := style
		if s.Status == "Running" {
			statusStyle = lipgloss.NewStyle().Foreground(colorPaleGreen)
			if i == m.cursor {
				statusStyle = statusStyle.Background(colorAqua).Foreground(colorGreen)
			}
		}

		row := style.Render(padRight(truncate(s.Name, nameW), nameW)) +
			style.Render(padRight(truncate(s.Server.URL, urlW), urlW)) +
			statusStyle.Render(padRight(truncate(s.Status, statusW), statusW)) +
			style.Render(padRight(clientsStr, clientsW))
		lines = append(lines, row)
	}

	for len(lines) < ch {
		lines = append(lines, strings.Repeat(" ", iw))
	}

	return m.renderBorderedBox(joinLines(lines), title, iw)
}

// ─── Server Detail View (3-panel layout) ────────────

func (m model) renderDetail() string {
	if m.cursor >= len(m.filtered) {
		return ""
	}
	ch := m.contentHeight()
	iw := m.innerWidth()

	topH := ch * 40 / 100
	if topH < 3 {
		topH = 3
	}
	bottomH := ch - topH - 2
	if bottomH < 3 {
		bottomH = 3
	}

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
		nameColW := iw * 30 / 100
		descColW := iw - nameColW
		topLines = append(topLines,
			tableHeaderStyle.Render(padRight("NAME", nameColW))+
				tableHeaderStyle.Render(padRight("DESCRIPTION", descColW)))

		dataRows := topH - 1
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
				style.Render(padRight(truncate(t.DisplayName(), nameColW), nameColW))+
					style.Render(padRight(truncate(t.Description, descColW), descColW)))
		}
	}
	topContent := m.padToHeight(joinLines(topLines), topH)
	topBox := m.renderBorderedBox(topContent, topTitle, iw)

	gap := 1
	bottomTotalInner := iw - 4 - gap
	leftInnerW := bottomTotalInner / 2
	rightInnerW := bottomTotalInner - leftInnerW

	leftTitle := tableTitleStyle.Render("Request")
	var leftLines []string
	if m.requestText != "" {
		for _, line := range splitLines(m.requestText) {
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
	leftContent := m.padToHeight(joinLines(leftLines), bottomH)
	leftBox := m.renderBorderedBox(leftContent, leftTitle, leftInnerW)

	rightTitle := tableTitleStyle.Render("Response")
	var rightLines []string
	if m.responseLoading {
		rightLines = append(rightLines, detailValueStyle.Render("Calling tool..."))
	} else if m.responseText != "" {
		m.responseLines = buildResponseLines(m.responseText, rightInnerW)
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
	rightContent := m.padToHeight(joinLines(rightLines), bottomH)
	rightBox := m.renderBorderedBox(rightContent, rightTitle, rightInnerW)

	bottomRow := joinHorizontal(leftBox, rightBox, gap)

	return topBox + "\n" + bottomRow
}

// ─── Crumbs ─────────────────────────────────────────

func (m model) renderCrumbs() string {
	var crumbs string
	if m.showToolDialog && m.dialogTool != nil {
		crumbs = crumbStyle.Render("servers") + " " +
			crumbStyle.Render(m.detailServerNm) + " " +
			crumbActiveStyle.Render(m.dialogTool.DisplayName())
	} else if m.view == viewDetail && m.cursor < len(m.filtered) {
		crumbs = crumbStyle.Render("servers") + " " +
			crumbActiveStyle.Render(m.filtered[m.cursor].Name)
	} else {
		crumbs = crumbActiveStyle.Render("servers")
	}

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
		tableTitleCountStyle.Render(tool.DisplayName()) +
		lipgloss.NewStyle().Foreground(colorAqua).Render("]")

	var lines []string

	if len(tool.Params) == 0 {
		lines = append(lines, "")
		lines = append(lines, detailValueStyle.Render("This tool has no parameters."))
		lines = append(lines, "")
	} else {
		for i, p := range tool.Params {
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

			border := fieldBorder
			if i == m.dialogParamIdx && !m.dialogOnOK {
				border = fieldBorderActive
			}
			lines = append(lines, border.Render(m.dialogFields[i].View()))
			lines = append(lines, "")
		}
	}

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

	lines = append(lines, "")
	hint := dimStyle.Render("tab/↓ next • shift-tab/↑ prev • enter confirm • esc cancel")
	hintW := lipgloss.Width(hint)
	hintPad := (iw - hintW) / 2
	if hintPad < 0 {
		hintPad = 0
	}
	lines = append(lines, strings.Repeat(" ", hintPad)+hint)

	content := m.padToHeight(joinLines(lines), ch)
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

	inner := joinLines(lines)
	maxW := m.width - 6
	if maxW < 20 {
		maxW = 20
	}
	return promptBorderCommandStyle.Padding(1, 2).MaxWidth(maxW).Render(inner)
}

func (m model) overlayCenter(bg, overlay string) string {
	bgLines := splitLines(bg)
	overlayLines := splitLines(overlay)

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

	return joinLines(bgLines)
}

// ─── Bordered Box ───────────────────────────────────

func (m model) renderBorderedBox(content, title string, innerWidth int) string {
	bc := lipgloss.NewStyle().Foreground(colorLightSkyBlue)

	tl, tr, bl, br := "╭", "╮", "╰", "╯"
	h, v := "─", "│"

	borderFill := innerWidth + 2

	titleStr := " " + title + " "
	titleVisualW := lipgloss.Width(titleStr)
	totalFill := borderFill - titleVisualW
	if totalFill < 0 {
		totalFill = 0
	}
	leftFill := totalFill / 2
	rightFill := totalFill - leftFill
	topBorder := bc.Render(tl+strings.Repeat(h, leftFill)) + titleStr + bc.Render(strings.Repeat(h, rightFill)+tr)

	bottomBorder := bc.Render(bl + strings.Repeat(h, borderFill) + br)

	contentLines := splitLines(content)
	var result []string
	result = append(result, topBorder)
	for _, line := range contentLines {
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

	return joinLines(result)
}
