package tui

import (
	"os/exec"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func splitLines(s string) []string {
	return strings.Split(s, "\n")
}

func joinLines(lines []string) string {
	return strings.Join(lines, "\n")
}

func truncate(s string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= maxW {
		return s
	}
	if maxW <= 3 {
		var out []rune
		w := 0
		for _, r := range s {
			rw := lipgloss.Width(string(r))
			if w+rw > maxW {
				break
			}
			out = append(out, r)
			w += rw
		}
		return string(out)
	}
	var out []rune
	w := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if w+rw > maxW-3 {
			break
		}
		out = append(out, r)
		w += rw
	}
	return string(out) + "..."
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
