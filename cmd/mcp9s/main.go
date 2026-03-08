package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chrisburns/mcp9s/internal/config"
	"github.com/chrisburns/mcp9s/internal/tui"
)

func main() {
	servers, clientCount := config.DiscoverServers()

	m := tui.NewModel(servers, clientCount)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}
