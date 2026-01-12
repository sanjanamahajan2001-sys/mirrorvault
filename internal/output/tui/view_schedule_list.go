package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m TUIModel) updateScheduleList(msg tea.Msg) (TUIModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up":
			if m.ScheduleIndex > 0 {
				m.ScheduleIndex--
			}
			return m, nil
		case "down":
			if m.ScheduleIndex < len(m.Schedules)-1 {
				m.ScheduleIndex++
			}
			return m, nil
		case "e":
			// Edit schedule time
			if len(m.Schedules) > 0 && m.ScheduleIndex < len(m.Schedules) {
				// Create a copy to avoid modifying the original
				schedCopy := m.Schedules[m.ScheduleIndex]
				m.ScheduleData = &schedCopy
				m.ScheduleTime = m.ScheduleData.Time
				m.ViewState = ViewScheduleTime
				return m, nil
			}
			return m, nil
		case "d":
			// Delete schedule - show confirmation
			if len(m.Schedules) > 0 && m.ScheduleIndex < len(m.Schedules) {
				if m.ScheduleIndex < len(m.ScheduleTimerNames) {
					m.PendingDeleteTimerName = m.ScheduleTimerNames[m.ScheduleIndex]
					m.ViewState = ViewScheduleDeleteConfirm
				}
			}
			return m, nil
		case "enter", "q", "ctrl+c":
			m.Exit = true
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m TUIModel) viewScheduleList() string {
	var b strings.Builder

	// If Mode is not set (for list-schedules command), use a default
	if m.Mode == 0 {
		b.WriteString(TitleStyle.Render("🗄  MirrorVault") + "\n")
		b.WriteString(SubtitleStyle.Render("Secure Database Backup Agent") + "\n\n")
	} else {
		renderHeader(&b, m.Mode)
	}

	b.WriteString(SectionTitleStyle.Render("All Scheduled Backups") + "\n\n")

	if len(m.Schedules) == 0 {
		b.WriteString("No scheduled backups found.\n\n")
	} else {
		for i, sched := range m.Schedules {
			// Add visual separator between schedules
			if i > 0 {
				b.WriteString("\n" + DividerStyle.Render(strings.Repeat("─", 44)) + "\n\n")
			}

			// Highlight selected schedule
			cursor := "  "
			style := ItemStyle
			if i == m.ScheduleIndex {
				cursor = "> "
				style = style.Foreground(lipgloss.Color("#89b4fa"))
			}

			b.WriteString(cursor + style.Render(fmt.Sprintf("Engine: %s", EngineNameStyle.Render(sched.Engine))) + "\n")
			b.WriteString("  " + style.Render(fmt.Sprintf("Databases: %s", strings.Join(sched.Databases, ", "))) + "\n")
			b.WriteString("  " + style.Render(fmt.Sprintf("Time: %s", sched.Time)) + "\n")
		}
	}

	b.WriteString("\n")

	// Show different footer based on whether schedules exist
	if len(m.Schedules) > 0 {
		b.WriteString(FooterStyle.Render(" ↑↓ navigate    E edit time    D delete    Enter/Ctrl+C exit "))
	} else {
		b.WriteString(FooterStyle.Render(" Press Enter to exit "))
	}

	return b.String()
}
