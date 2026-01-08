package tui

import "github.com/charmbracelet/lipgloss"

var (
	TitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#89b4fa"))

	SubtitleStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#bac2de"))

	ModeStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#f9e2af"))

	SectionTitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#94e2d5"))

	TileStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#6c7086")).
		Padding(0, 3).
		Width(30).
		Align(lipgloss.Center).
		MarginRight(1).
		MarginBottom(1)

	EngineNameStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#cdd6f4"))

	EngineStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#cdd6f4")).
		Underline(true)

	AuthStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#f38ba8"))

	NoAuthStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#a6e3a1"))

	ItemStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#bac2de"))

	DividerStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#585b70"))

	FooterStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#1e1e2e")).
		Background(lipgloss.Color("#89b4fa")).
		Padding(0, 1).
		Bold(true)

	KeyHintStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#eff1f5")).
		Background(lipgloss.Color("#45475a")).
		Padding(0, 1)

	// ✅ NEW: Summary / handoff box
	SummaryBoxStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#89b4fa")).
		Padding(1, 3).
		Width(50).
		Align(lipgloss.Left)
)
