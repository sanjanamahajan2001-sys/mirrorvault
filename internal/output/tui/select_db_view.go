package tui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"strings"

	"mirrorvault/internal/backup/plan"
	"mirrorvault/pkg/model"
)

const allDatabasesLabel = "Backup all databases"

func dbSelectOptions(engine *model.Database) []string {
	if engine == nil {
		return nil
	}
	displayNames := filterDefaultDatabases(engine.Engine, engine.Names)
	options := make([]string, 0, len(displayNames)+1)
	options = append(options, displayNames...)
	options = append(options, allDatabasesLabel)
	return options
}

func (m TUIModel) updateDBSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	engine := m.currentEngine()
	if engine == nil {
		m.ViewState = ViewSelectEngine
		return m, nil
	}

	displayNames := dbSelectOptions(engine)

	switch msg.String() {
	case "up":
		if m.Selection.DBIndex > 0 {
			m.Selection.DBIndex--
			// Adjust scroll to keep selected item visible
			m.adjustDBSelectScroll(displayNames, engine)
		}
	case "down":
		// Use filtered names for display, but check against full list
		if m.Selection.DBIndex < len(displayNames)-1 {
			m.Selection.DBIndex++
			// Adjust scroll to keep selected item visible
			m.adjustDBSelectScroll(displayNames, engine)
		}
	case " ":
		// Get the actual database name from the filtered display list
		displayNames := dbSelectOptions(engine)
		if m.Selection.DBIndex >= 0 && m.Selection.DBIndex < len(displayNames) {
			name := displayNames[m.Selection.DBIndex]
			engineName := engine.Engine

			// Initialize engine map if it doesn't exist
			if m.Selection.SelectedDBs[engineName] == nil {
				m.Selection.SelectedDBs[engineName] = make(map[string]bool)
			}

			selectionKey := name
			if name == allDatabasesLabel {
				if engine.RequiresAuth {
					return m, nil
				}
				selectionKey = plan.AllDatabasesName
			}

			if selectionKey == plan.AllDatabasesName {
				if m.Selection.SelectedDBs[engineName][selectionKey] {
					delete(m.Selection.SelectedDBs[engineName], selectionKey)
				} else {
					m.Selection.SelectedDBs[engineName] = map[string]bool{selectionKey: true}
				}
				return m, nil
			}

			if m.Selection.SelectedDBs[engineName][plan.AllDatabasesName] {
				delete(m.Selection.SelectedDBs[engineName], plan.AllDatabasesName)
			}

			if engine.RequiresAuth {
				// For auth-enabled engines, only one database allowed
				// Clear all selections for this engine first
				m.Selection.SelectedDBs[engineName] = map[string]bool{selectionKey: true}
			} else {
				// Toggle selection for this database in this engine
				m.Selection.SelectedDBs[engineName][selectionKey] = !m.Selection.SelectedDBs[engineName][selectionKey]
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

// adjustDBSelectScroll adjusts the scroll offset to keep the selected item visible
func (m *TUIModel) adjustDBSelectScroll(displayNames []string, engine *model.Database) {
	// Calculate available height for database list
	// Reserve space for: header (2 lines), auth message (2 lines if shown), footer (1 line)
	headerHeight := 2
	footerHeight := 1
	authMessageHeight := 0
	if engine != nil && engine.RequiresAuth {
		authMessageHeight = 2
	}
	availableHeight := m.TerminalHeight - headerHeight - footerHeight - authMessageHeight - 1
	if availableHeight < 1 {
		availableHeight = 1
	}

	maxScroll := len(displayNames) - availableHeight
	if maxScroll < 0 {
		maxScroll = 0
	}

	// Ensure selected item is visible
	if m.Selection.DBIndex < m.DBSelectScrollOffset {
		m.DBSelectScrollOffset = m.Selection.DBIndex
	} else if m.Selection.DBIndex >= m.DBSelectScrollOffset+availableHeight {
		m.DBSelectScrollOffset = m.Selection.DBIndex - availableHeight + 1
	}

	// Clamp scroll offset
	if m.DBSelectScrollOffset > maxScroll {
		m.DBSelectScrollOffset = maxScroll
	}
	if m.DBSelectScrollOffset < 0 {
		m.DBSelectScrollOffset = 0
	}
}

func (m TUIModel) viewDBSelect() string {
	engine := m.currentEngine()
	if engine == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString(SectionTitleStyle.Render(fmt.Sprintf("Select database(s) for %s", engine.Engine)) + "\n\n")

	displayNames := dbSelectOptions(engine)

	// Calculate available height for database list
	// Reserve space for: header (2 lines), auth message (2 lines if shown), footer (1 line)
	headerHeight := 2
	footerHeight := 1
	authMessageHeight := 0
	if engine.RequiresAuth {
		authMessageHeight = 2
	}
	availableHeight := m.TerminalHeight - headerHeight - footerHeight - authMessageHeight - 1 // -1 for safety margin

	// Ensure available height is at least 1
	if availableHeight < 1 {
		availableHeight = 1
	}

	// Adjust scroll offset (this ensures it's always valid)
	maxScroll := len(displayNames) - availableHeight
	if maxScroll < 0 {
		maxScroll = 0
	}

	// Clamp scroll offset to valid range
	if m.DBSelectScrollOffset > maxScroll {
		m.DBSelectScrollOffset = maxScroll
	}
	if m.DBSelectScrollOffset < 0 {
		m.DBSelectScrollOffset = 0
	}

	// Determine which items to show
	startIdx := m.DBSelectScrollOffset
	endIdx := startIdx + availableHeight
	if endIdx > len(displayNames) {
		endIdx = len(displayNames)
	}

	// Render visible items
	for i := startIdx; i < endIdx; i++ {
		name := displayNames[i]
		// Strict spacing: 2 for cursor, 4 for checkbox
		cursor := "  "
		if i == m.Selection.DBIndex {
			cursor = "> "
		}

		check := "[ ] "
		engineName := engine.Engine
		selectionKey := name
		if name == allDatabasesLabel {
			selectionKey = plan.AllDatabasesName
		}
		if m.Selection.SelectedDBs[engineName] != nil && m.Selection.SelectedDBs[engineName][selectionKey] {
			check = "[x] "
		}

		displayName := name
		if name == allDatabasesLabel && engine.RequiresAuth {
			displayName = "Backup all databases (disabled - auth enabled)"
		}
		b.WriteString(fmt.Sprintf("%s%s%s\n", cursor, check, displayName))
	}

	if engine.RequiresAuth {
		b.WriteString("\n" + AuthStyle.Render("! Auth enabled: create separate backups per database") + "\n")
	}

	// Add scroll indicator if needed
	var footerText string
	if len(displayNames) > availableHeight {
		footerText = fmt.Sprintf(" ↑/↓ move • Space select • Enter confirm • Esc back • Ctrl+C exit [%d/%d]", m.Selection.DBIndex+1, len(displayNames))
	} else {
		footerText = " ↑/↓ move • Space select • Enter confirm • Esc back • Ctrl+C exit "
	}

	b.WriteString("\n" + FooterStyle.Render(footerText))
	return b.String()
}
