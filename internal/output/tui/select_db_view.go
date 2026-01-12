package tui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"strings"
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
			engineName := engine.Engine

			// Initialize engine map if it doesn't exist
			if m.Selection.SelectedDBs[engineName] == nil {
				m.Selection.SelectedDBs[engineName] = make(map[string]bool)
			}

			if engine.RequiresAuth {
				// For auth-enabled engines, only one database allowed
				// Clear all selections for this engine first
				m.Selection.SelectedDBs[engineName] = map[string]bool{name: true}
			} else {
				// Toggle selection for this database in this engine
				m.Selection.SelectedDBs[engineName][name] = !m.Selection.SelectedDBs[engineName][name]
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
	if engine == nil {
		return ""
	}

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
		engineName := engine.Engine
		if m.Selection.SelectedDBs[engineName] != nil && m.Selection.SelectedDBs[engineName][name] {
			check = "[x] "
		}

		b.WriteString(fmt.Sprintf("%s%s%s\n", cursor, check, name))
	}

	if engine.RequiresAuth {
		b.WriteString("\n" + AuthStyle.Render("! Auth enabled: only ONE database allowed") + "\n")
	}

	b.WriteString("\n" + FooterStyle.Render(" ↑/↓ move • Space select • Enter confirm • Esc back • Ctrl+C exit "))
	return b.String()
}
