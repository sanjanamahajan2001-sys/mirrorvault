package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
)

func (m TUIModel) viewRestoreHistory() string {
	var b strings.Builder
	b.WriteString(SectionTitleStyle.Render("Restore History") + "\n\n")

	if len(m.RestoreHistory) == 0 {
		b.WriteString(ItemStyle.Render("No restore operations found.") + "\n\n")
		b.WriteString(FooterStyle.Render("Press Esc to go back • Ctrl+C to exit"))
		return b.String()
	}

	// Color styles
	successStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#a6e3a1")).
		Bold(true)
	failureStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#f38ba8")).
		Bold(true)
	engineStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#89b4fa")).
		Bold(true)
	dbStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#a6e3a1"))
	infoStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#cdd6f4"))
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#f9e2af")).
		Bold(true)

	// Calculate available height for scrolling
	availableHeight := m.TerminalHeight - 5 // Reserve space for header and footer
	if availableHeight < 10 {
		availableHeight = 10
	}

	// Determine which items to show based on scroll offset
	startIdx := m.RestoreHistoryScrollOffset
	endIdx := startIdx + availableHeight/8 // Each restore takes about 8 lines
	if endIdx > len(m.RestoreHistory) {
		endIdx = len(m.RestoreHistory)
	}

	// Show items
	for i := startIdx; i < endIdx; i++ {
		history := m.RestoreHistory[i]
		
		// Visual separator between restorations
		if i > startIdx {
			b.WriteString("\n" + DividerStyle.Render(strings.Repeat("═", 70)) + "\n\n")
		}

		// Status indicator
		statusText := successStyle.Render("✓ SUCCESS")
		if !history.Success {
			statusText = failureStyle.Render("✗ FAILED")
		}
		if history.RolledBack {
			statusText += " " + lipgloss.NewStyle().Foreground(lipgloss.Color("#fab387")).Bold(true).Render("(ROLLED BACK)")
		}

		// Main info box
		infoBoxWidth := 70
		infoContentWidth := 60
		
		topBorder := "┌" + strings.Repeat("─", infoBoxWidth-2) + "┐"
		bottomBorder := "└" + strings.Repeat("─", infoBoxWidth-2) + "┘"
		
		b.WriteString(topBorder + "\n")
		b.WriteString(fmt.Sprintf("│ %s │\n", padString(headerStyle.Render(fmt.Sprintf("Restore #%d - %s", i+1, history.Timestamp)), infoContentWidth)))
		b.WriteString("├" + strings.Repeat("─", infoBoxWidth-2) + "┤\n")
		
		// Engine and Database
		engineText := engineStyle.Render(history.Engine)
		dbText := dbStyle.Render(history.Database)
		b.WriteString(fmt.Sprintf("│ %s │\n", padString(fmt.Sprintf("Engine: %s", engineText), infoContentWidth)))
		b.WriteString(fmt.Sprintf("│ %s │\n", padString(fmt.Sprintf("Database: %s", dbText), infoContentWidth)))
		b.WriteString("├" + strings.Repeat("─", infoBoxWidth-2) + "┤\n")
		
		// Status
		b.WriteString(fmt.Sprintf("│ %s │\n", padString(fmt.Sprintf("Status: %s", statusText), infoContentWidth)))
		b.WriteString("├" + strings.Repeat("─", infoBoxWidth-2) + "┤\n")
		
		// Dump information
		dumpPath := history.DumpPath
		if len(dumpPath) > 50 {
			dumpPath = "..." + dumpPath[len(dumpPath)-47:]
		}
		b.WriteString(fmt.Sprintf("│ %s │\n", padString(fmt.Sprintf("Dump Path: %s", infoStyle.Render(dumpPath)), infoContentWidth)))
		
		formatInfo := history.DumpFormat
		if history.Compressed {
			formatInfo += " (compressed)"
		}
		if history.MultiDB {
			formatInfo += " (multi-DB)"
		}
		b.WriteString(fmt.Sprintf("│ %s │\n", padString(fmt.Sprintf("Format: %s", infoStyle.Render(formatInfo)), infoContentWidth)))
		b.WriteString("├" + strings.Repeat("─", infoBoxWidth-2) + "┤\n")
		
		// Pre-restore backup
		if history.PreRestoreBackup != "" {
			backupPath := history.PreRestoreBackup
			if len(backupPath) > 50 {
				backupPath = "..." + backupPath[len(backupPath)-47:]
			}
			b.WriteString(fmt.Sprintf("│ %s │\n", padString(fmt.Sprintf("Pre-Restore Backup: %s", infoStyle.Render(backupPath)), infoContentWidth)))
		}
		
		// Error if failed
		if !history.Success && history.Error != "" {
			errorMsg := history.Error
			if len(errorMsg) > 60 {
				errorMsg = errorMsg[:57] + "..."
			}
			b.WriteString("├" + strings.Repeat("─", infoBoxWidth-2) + "┤\n")
			b.WriteString(fmt.Sprintf("│ %s │\n", padString(fmt.Sprintf("Error: %s", failureStyle.Render(errorMsg)), infoContentWidth)))
		}
		
		b.WriteString(bottomBorder + "\n")
	}

	// Scroll indicator
	if len(m.RestoreHistory) > endIdx-startIdx {
		scrollPercent := int(float64(m.RestoreHistoryScrollOffset) / float64(len(m.RestoreHistory)) * 100)
		if scrollPercent > 100 {
			scrollPercent = 100
		}
		b.WriteString("\n")
		b.WriteString(FooterStyle.Render(fmt.Sprintf("[Scroll: %d%% ↑/k up ↓/j down] ", scrollPercent)))
	}

	b.WriteString(FooterStyle.Render("Esc back • Enter view details • Ctrl+C exit"))
	return b.String()
}

func (m TUIModel) updateRestoreHistory(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Go back to restore mode start
		m.ViewState = ViewRestoreSelectEngine
		m.RestoreHistoryScrollOffset = 0
		return m, nil
	case "up", "k":
		if m.RestoreHistoryScrollOffset > 0 {
			m.RestoreHistoryScrollOffset--
		}
		return m, nil
	case "down", "j":
		availableHeight := m.TerminalHeight - 5
		if availableHeight < 10 {
			availableHeight = 10
		}
		maxOffset := len(m.RestoreHistory) - (availableHeight / 8)
		if maxOffset < 0 {
			maxOffset = 0
		}
		if m.RestoreHistoryScrollOffset < maxOffset {
			m.RestoreHistoryScrollOffset++
		}
		return m, nil
	case "enter":
		// Could show detailed view in future
		return m, nil
	}
	return m, nil
}
