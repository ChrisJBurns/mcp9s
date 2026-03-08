package tui

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// buildResponseLines pretty-prints JSON (if valid) with syntax highlighting,
// then word-wraps to fit the given width. Handles SSE "data: {...}" lines.
func buildResponseLines(text string, width int) []string {
	display := extractAndFormatJSON(text)

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
	var buf json.RawMessage
	if json.Unmarshal([]byte(text), &buf) == nil {
		if pretty, err := json.MarshalIndent(buf, "", "  "); err == nil {
			return string(pretty)
		}
	}

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
			otherParts = append(otherParts, line)
		} else if trimmed == "" {
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

func highlightJSONLine(line string) string {
	keyStyle := lipgloss.NewStyle().Foreground(colorDodgerBlue).Bold(true)
	strStyle := lipgloss.NewStyle().Foreground(colorPaleGreen)
	numStyle := lipgloss.NewStyle().Foreground(colorFuchsia)
	boolStyle := lipgloss.NewStyle().Foreground(colorDarkOrange).Bold(true)
	nullStyle := lipgloss.NewStyle().Foreground(colorLightSlateGray)
	punctStyle := lipgloss.NewStyle().Foreground(colorLightSkyBlue)

	if m := jsonLineRe.FindStringSubmatch(line); m != nil {
		indent := m[1]
		k := m[2]
		v := strings.TrimRight(m[3], " ")
		return indent + keyStyle.Render(k) + punctStyle.Render(": ") + colorJSONValue(v, strStyle, numStyle, boolStyle, nullStyle, punctStyle)
	}

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
