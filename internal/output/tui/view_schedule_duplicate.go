package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"mirrorvault/internal/schedule"
)

func (m TUIModel) updateScheduleDuplicate(msg tea.Msg) (TUIModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up":
			if m.ScheduleIndex > 0 {
				m.ScheduleIndex--
			}
			return m, nil
		case "down":
			if m.ScheduleIndex < len(m.DuplicateSchedules)-1 {
				m.ScheduleIndex++
			}
			return m, nil
		case "e":
			// Edit schedule time
			if len(m.DuplicateSchedules) > 0 && m.ScheduleIndex < len(m.DuplicateSchedules) {
				// Create a copy to avoid modifying the original
				schedCopy := m.DuplicateSchedules[m.ScheduleIndex]
				m.ScheduleData = &schedCopy
				m.ScheduleTime = m.ScheduleData.Time
				// Store the timer name temporarily in ScheduleTimerNames for the update flow
				if m.ScheduleIndex < len(m.DuplicateTimerNames) {
					m.ScheduleTimerNames = []string{m.DuplicateTimerNames[m.ScheduleIndex]}
					m.ScheduleIndex = 0
				}
				m.ViewState = ViewScheduleTime
				return m, nil
			}
			return m, nil
		case "d":
			// Delete schedule - show confirmation
			if len(m.DuplicateSchedules) > 0 && m.ScheduleIndex < len(m.DuplicateSchedules) {
				if m.ScheduleIndex < len(m.DuplicateTimerNames) {
					m.PendingDeleteTimerName = m.DuplicateTimerNames[m.ScheduleIndex]
					m.ViewState = ViewScheduleDeleteConfirm
					return m, nil
				}
			}
			return m, nil
		case "esc":
			// Go back to time input
			m.ViewState = ViewScheduleTime
			return m, nil
		case "q", "ctrl+c":
			m.Exit = true
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m TUIModel) viewScheduleDuplicate() string {
	var b strings.Builder

	renderHeader(&b, m.Mode)
	b.WriteString(SectionTitleStyle.Render("Duplicate Schedule Detected") + "\n\n")

	b.WriteString(AuthStyle.Render("⚠ Warning: Backup already scheduled for:\n"))
	if m.ScheduleData != nil {
		duplicates, _ := schedule.CheckDuplicate(m.ScheduleData.Engine, m.ScheduleData.Databases)
		for _, db := range duplicates {
			b.WriteString(fmt.Sprintf("  - %s\n", db))
		}
	}
	b.WriteString("\n")

	b.WriteString("Existing schedules:\n\n")

	if len(m.DuplicateSchedules) == 0 {
		b.WriteString("No conflicting schedules found.\n\n")
	} else {
		for i, sched := range m.DuplicateSchedules {
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
			b.WriteString("  " + style.Render(fmt.Sprintf("Databases: %s", formatDatabaseList(sched.Databases))) + "\n")
			b.WriteString("  " + style.Render(fmt.Sprintf("Time: %s", sched.Time)) + "\n")
			formatLabel := "Native"
			if sched.Compression != "" {
				formatLabel = fmt.Sprintf("Compressed (%s)", sched.Compression)
			}
			b.WriteString("  " + style.Render(fmt.Sprintf("Format: %s", formatLabel)) + "\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(FooterStyle.Render(" ↑↓ navigate    E edit time    D delete    ESC back    Ctrl+C exit "))

	return b.String()
}
