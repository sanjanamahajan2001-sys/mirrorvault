package tui

import (
	"fmt"
	"strings"
	tea "github.com/charmbracelet/bubbletea"
)

func (m TUIModel) updateDBSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	engine := m.currentEngine()
	if engine == nil {
		m.ViewState = ViewSelectEngine
		return m, nil
	}

	switch msg.String() {
	case "up":
		if m.Selection.DBIndex > 0 {
			m.Selection.DBIndex--
		}
	case "down":
		// Use filtered names for display, but check against full list
		displayNames := filterDefaultDatabases(engine.Engine, engine.Names)
		if m.Selection.DBIndex < len(displayNames)-1 {
			m.Selection.DBIndex++
		}
	case " ":
		// Get the actual database name from the filtered display list
		displayNames := filterDefaultDatabases(engine.Engine, engine.Names)
		if m.Selection.DBIndex >= 0 && m.Selection.DBIndex < len(displayNames) {
			name := displayNames[m.Selection.DBIndex]
			if engine.RequiresAuth {
				m.Selection.SelectedDBs = map[string]bool{name: true}
			} else {
				m.Selection.SelectedDBs[name] = !m.Selection.SelectedDBs[name]
			}
		}
	case "enter":
		m.Ready = true
		return m, tea.Quit
	case "esc":
		m.ViewState = ViewSelectEngine
	}
	return m, nil
}

func (m TUIModel) viewDBSelect() string {
	engine := m.currentEngine()
	if engine == nil { return "" }

	var b strings.Builder
	b.WriteString(SectionTitleStyle.Render(fmt.Sprintf("Select database(s) for %s", engine.Engine)) + "\n\n")

	// Filter default databases for display (but keep them available for backup)
	displayNames := filterDefaultDatabases(engine.Engine, engine.Names)

	for i, name := range displayNames {
		// Strict spacing: 2 for cursor, 4 for checkbox
		cursor := "  " 
		if i == m.Selection.DBIndex {
			cursor = "> "
		}

		check := "[ ] "
		if m.Selection.SelectedDBs[name] {
			check = "[x] "
		}

		b.WriteString(fmt.Sprintf("%s%s%s\n", cursor, check, name))
	}

	if engine.RequiresAuth {
		b.WriteString("\n" + AuthStyle.Render("! Auth enabled: only ONE database allowed") + "\n")
	}

	b.WriteString("\n" + FooterStyle.Render(" ↑/↓ move • Space select • Enter confirm • Esc back • Q exit "))
	return b.String()
}
