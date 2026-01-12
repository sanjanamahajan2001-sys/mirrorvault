package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"mirrorvault/internal/schedule"
)

func (m TUIModel) updateScheduleDeleteConfirm(msg tea.Msg) (TUIModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", "y":
			// Confirm deletion
			if m.PendingDeleteTimerName != "" {
				// Remove the schedule
				if err := schedule.RemoveSchedule(m.PendingDeleteTimerName); err != nil {
					// Error - go back to previous view
					if len(m.DuplicateSchedules) > 0 {
						m.ViewState = ViewScheduleDuplicate
					} else {
						m.ViewState = ViewScheduleList
					}
					m.PendingDeleteTimerName = ""
					return m, nil
				}

				// Reload schedules
				allSchedules, err := schedule.GetAllSchedules()
				if err != nil {
					allSchedules = []schedule.Schedule{}
				}

				// Update model
				m.Schedules = []ScheduleData{}
				m.ScheduleTimerNames = []string{}
				for _, s := range allSchedules {
					m.Schedules = append(m.Schedules, ScheduleData{
						Engine:    s.Engine,
						Databases: s.Databases,
						Time:      s.Time,
					})
					m.ScheduleTimerNames = append(m.ScheduleTimerNames, s.TimerName)
				}

				// Clear duplicate data and go to list view
				m.DuplicateSchedules = []ScheduleData{}
				m.DuplicateTimerNames = []string{}
				m.PendingDeleteTimerName = ""
				m.ViewState = ViewScheduleList
				m.ScheduleIndex = 0
			}
			return m, nil
		case "esc", "n":
			// Cancel deletion
			m.PendingDeleteTimerName = ""
			if len(m.DuplicateSchedules) > 0 {
				m.ViewState = ViewScheduleDuplicate
			} else {
				m.ViewState = ViewScheduleList
			}
			return m, nil
		case "q", "ctrl+c":
			m.Exit = true
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m TUIModel) viewScheduleDeleteConfirm() string {
	var b strings.Builder

	renderHeader(&b, m.Mode)
	b.WriteString(SectionTitleStyle.Render("Confirm Deletion") + "\n\n")

	b.WriteString(AuthStyle.Render("⚠ Warning: This will permanently delete the schedule!") + "\n\n")

	// Find the schedule being deleted
	var scheduleToDelete *ScheduleData
	if m.PendingDeleteTimerName != "" {
		// Try to find in duplicate schedules first
		for i, timerName := range m.DuplicateTimerNames {
			if timerName == m.PendingDeleteTimerName && i < len(m.DuplicateSchedules) {
				schedCopy := m.DuplicateSchedules[i]
				scheduleToDelete = &schedCopy
				break
			}
		}
		// If not found, try in regular schedules
		if scheduleToDelete == nil {
			for i, timerName := range m.ScheduleTimerNames {
				if timerName == m.PendingDeleteTimerName && i < len(m.Schedules) {
					schedCopy := m.Schedules[i]
					scheduleToDelete = &schedCopy
					break
				}
			}
		}
	}

	if scheduleToDelete != nil {
		// Write as plain text to ensure left alignment - ensure newline is outside any style
		b.WriteString("\nSchedule to delete:\n\n")

		b.WriteString(fmt.Sprintf("Engine: %s\n", EngineNameStyle.Render(scheduleToDelete.Engine)))
		b.WriteString(fmt.Sprintf("Databases: %s\n", strings.Join(scheduleToDelete.Databases, ", ")))
		b.WriteString(fmt.Sprintf("Time: %s\n\n", scheduleToDelete.Time))
		b.WriteString("This will stop and remove the scheduled backup.\n")
		b.WriteString("Backup files will NOT be deleted.\n\n")
	} else {
		b.WriteString("Schedule information not available.\n\n")
	}

	b.WriteString(FooterStyle.Render(" Enter/Y confirm    ESC/N cancel    Ctrl+C exit "))

	return b.String()
}
