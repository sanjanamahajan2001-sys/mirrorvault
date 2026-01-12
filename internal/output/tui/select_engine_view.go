package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------- UPDATE LOGIC (UNCHANGED BEHAVIOR) ----------------

func (m TUIModel) updateEngineSelect(msg tea.KeyMsg) (TUIModel, tea.Cmd) {
	switch msg.String() {
	case "up":
		if m.Selection.EngineIndex > 0 {
			m.Selection.EngineIndex--
		}
	case "down":
		if m.Selection.EngineIndex < len(m.ScanResult.Databases)-1 {
			m.Selection.EngineIndex++
		}
	case "enter":
		m.ViewState = ViewSelectDB
		m.DBSelectScrollOffset = 0 // Reset scroll when entering DB selection
		m.Selection.DBIndex = 0    // Reset selection index
	case "q", "ctrl+c":
		m.Exit = true
		return m, tea.Quit
	}
	return m, nil
}

// ---------------- VIEW LOGIC (FIXED PRESENTATION) ----------------

func (m TUIModel) viewEngineSelect() string {
	var b strings.Builder

	// Header — keep consistent with scan screen
	renderHeader(&b, m.Mode)

	var title string
	if m.Mode == ScheduleMode {
		title = "Select database engine for daily backup"
	} else {
		title = "Select Database Engine"
	}
	b.WriteString(SectionTitleStyle.Render(title) + "\n\n")

	for i, db := range m.ScanResult.Databases {
		// Base style
		style := TileStyle.Copy()

		cursor := "  "
		if i == m.Selection.EngineIndex {
			style = style.BorderForeground(lipgloss.Color("#89b4fa"))
			cursor = "> "
		}

		authLabel := NoAuthStyle.Render("No auth")
		if db.RequiresAuth {
			authLabel = AuthStyle.Render("Auth Required")
		}

		// ✅ NORMALIZED VERSION (important fix)
		displayVersion := normalizeVersion(db.Engine, db.Version)

		content := lipgloss.JoinVertical(
			lipgloss.Center,
			EngineNameStyle.Render(db.Engine+" "+displayVersion),
			authLabel,
		)

		tile := style.Render(content)
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Center, cursor, tile) + "\n")
	}

	b.WriteString("\n" + FooterStyle.Render(" ↑ ↓ move    Enter select    Ctrl+C exit "))

	return b.String()
}
