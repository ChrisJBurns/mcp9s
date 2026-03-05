package main

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	Top      key.Binding
	Bottom   key.Binding
	Describe key.Binding
	Enter    key.Binding
	Filter   key.Binding
	Command  key.Binding
	Help     key.Binding
	Back     key.Binding
	Quit     key.Binding
	ForceQ   key.Binding
	Copy       key.Binding
	Exec       key.Binding
	ScrollUp   key.Binding
	ScrollDown key.Binding
}

var keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "Up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "Down"),
	),
	Top: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("g", "Top"),
	),
	Bottom: key.NewBinding(
		key.WithKeys("G"),
		key.WithHelp("G", "Bottom"),
	),
	Describe: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "Describe"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "Describe"),
	),
	Filter: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "Filter"),
	),
	Command: key.NewBinding(
		key.WithKeys(":"),
		key.WithHelp(":", "Command"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "Help"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "Back"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q"),
		key.WithHelp("q", "Quit"),
	),
	ForceQ: key.NewBinding(
		key.WithKeys("ctrl+c"),
	),
	Copy: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "Copy request"),
	),
	Exec: key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "Execute"),
	),
	ScrollUp: key.NewBinding(
		key.WithKeys("["),
		key.WithHelp("[/]", "Scroll response"),
	),
	ScrollDown: key.NewBinding(
		key.WithKeys("]"),
	),
}

// hintBindings returns the key hints displayed in the header menu area.
func hintBindings(detail bool) []key.Binding {
	if detail {
		return []key.Binding{
			keys.Back,
			keys.Enter,
			keys.Copy,
			keys.Exec,
			keys.ScrollUp,
			keys.Quit,
		}
	}
	return []key.Binding{
		keys.Quit,
		keys.Up,
		keys.Down,
		keys.Top,
		keys.Bottom,
		keys.Describe,
		keys.Enter,
		keys.Filter,
		keys.Command,
		keys.Help,
	}
}
