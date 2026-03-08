package tui

import "github.com/charmbracelet/lipgloss"

// k9s exact default color palette
var (
	colorBlack          = lipgloss.Color("#000000")
	colorWhite          = lipgloss.Color("#FFFFFF")
	colorAqua           = lipgloss.Color("#00FFFF")
	colorOrange         = lipgloss.Color("#FFA500")
	colorDodgerBlue     = lipgloss.Color("#1E90FF")
	colorLightSkyBlue   = lipgloss.Color("#87CEFA")
	colorFuchsia        = lipgloss.Color("#FF00FF")
	colorPapayaWhip     = lipgloss.Color("#FFEFD5")
	colorSeaGreen       = lipgloss.Color("#2E8B57")
	colorCadetBlue      = lipgloss.Color("#5F9EA0")
	colorGreen          = lipgloss.Color("#008000")
	colorPaleGreen      = lipgloss.Color("#98FB98")
	colorOrangeRed      = lipgloss.Color("#FF4500")
	colorDarkOrange     = lipgloss.Color("#FF8C00")
	colorLightSlateGray = lipgloss.Color("#778899")
)

// Logo styles
var (
	logoStyle = lipgloss.NewStyle().
			Foreground(colorOrange).
			Bold(true)

	logoStatusStyle = lipgloss.NewStyle().
			Foreground(colorGreen).
			Bold(true)
)

// Info panel (top-left cluster info analog)
var (
	infoLabelStyle = lipgloss.NewStyle().
			Foreground(colorOrange).
			Bold(true)

	infoValueStyle = lipgloss.NewStyle().
			Foreground(colorWhite).
			Bold(true)
)

// Menu / hint bar (in header, center)
var (
	menuKeyStyle = lipgloss.NewStyle().
			Foreground(colorDodgerBlue).
			Bold(true)

	menuDescStyle = lipgloss.NewStyle().
			Foreground(colorWhite)

	menuNumKeyStyle = lipgloss.NewStyle().
			Foreground(colorFuchsia).
			Bold(true)
)

// Table border and title
var (
	tableBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorDodgerBlue)

	tableBorderFocusedStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorLightSkyBlue)

	tableTitleStyle = lipgloss.NewStyle().
			Foreground(colorAqua).
			Bold(true)

	tableTitleCountStyle = lipgloss.NewStyle().
				Foreground(colorPapayaWhip).
				Bold(true)

	tableTitleFilterStyle = lipgloss.NewStyle().
				Foreground(colorSeaGreen)

	tableHeaderStyle = lipgloss.NewStyle().
				Foreground(colorWhite).
				Bold(false)

	tableRowStyle = lipgloss.NewStyle().
			Foreground(colorAqua)

	tableSelectedStyle = lipgloss.NewStyle().
				Foreground(colorBlack).
				Background(colorAqua).
				Bold(true)
)

// Crumbs (bottom bar, above flash)
var (
	crumbStyle = lipgloss.NewStyle().
			Foreground(colorBlack).
			Background(colorAqua).
			Bold(true).
			Padding(0, 1)

	crumbActiveStyle = lipgloss.NewStyle().
				Foreground(colorBlack).
				Background(colorOrange).
				Bold(true).
				Padding(0, 1)
)

// Prompt (command/filter bar)
var (
	promptBorderCommandStyle = lipgloss.NewStyle().
					Border(lipgloss.RoundedBorder()).
					BorderForeground(colorAqua)

	promptBorderFilterStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorSeaGreen)

	promptTextStyle = lipgloss.NewStyle().
			Foreground(colorCadetBlue)

	promptSuggestStyle = lipgloss.NewStyle().
				Foreground(colorDodgerBlue)
)

// Detail / describe view (YAML-like)
var (
	detailKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#4682B4")). // steelblue
			Bold(true)

	detailColonStyle = lipgloss.NewStyle().
				Foreground(colorWhite)

	detailValueStyle = lipgloss.NewStyle().
				Foreground(colorPapayaWhip)
)

// Flash / status bar
var (
	flashInfoStyle = lipgloss.NewStyle().
			Foreground(colorGreen).
			Bold(true)
)
