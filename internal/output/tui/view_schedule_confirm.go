package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"mirrorvault/internal/schedule"
)

func (m TUIModel) updateScheduleConfirm(msg tea.Msg) (TUIModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			// Confirm and create/update schedule
			if m.ScheduleData == nil {
				return m, nil
			}

			// Check if we're editing an existing schedule (from list view)
			isEditing := len(m.ScheduleTimerNames) > 0 && m.ScheduleIndex >= 0 && m.ScheduleIndex < len(m.ScheduleTimerNames)
			
			if isEditing {
				// Update existing schedule
				timerName := m.ScheduleTimerNames[m.ScheduleIndex]
				err := schedule.UpdateScheduleTime(timerName, m.ScheduleData.Time)
				if err != nil {
					// Error updating - stay on confirmation view
					return m, nil
				}
			} else {
				// Creating new schedule - check for duplicates
				duplicates, err := schedule.CheckDuplicate(m.ScheduleData.Engine, m.ScheduleData.Databases)
				if err != nil {
					// Error checking, but proceed anyway
				} else if len(duplicates) > 0 {
					// Store duplicate info in model to show message
					// For now, we'll show error and allow going back
					return m, nil
				}

				// Add schedule
				err = schedule.AddSchedule(
					m.ScheduleData.Engine,
					m.ScheduleData.Databases,
					m.ScheduleData.Time,
				)

				if err != nil {
					// If it's a duplicate error, show it
					if strings.Contains(err.Error(), "already scheduled") {
						// Store error to display
						return m, nil
					}
					// Other errors - for now just return
					return m, nil
				}
			}

			// Success - load all schedules and show list
			allSchedules, err := schedule.GetAllSchedules()
			if err != nil {
				allSchedules = []schedule.Schedule{}
			}

			// Convert to ScheduleData format for display
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

			m.ViewState = ViewScheduleList
			m.ScheduleIndex = 0 // Reset to first item
			return m, nil

		case "esc":
			// Go back to time input
			m.ViewState = ViewScheduleTime
			if m.ScheduleData != nil {
				m.ScheduleTime = m.ScheduleData.Time // Restore original time
			}
			return m, nil

		case "q", "ctrl+c":
			m.Exit = true
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m TUIModel) viewScheduleConfirm() string {
	var b strings.Builder

	renderHeader(&b, m.Mode)

	// Check if we're editing or creating
	isEditing := len(m.ScheduleTimerNames) > 0 && m.ScheduleIndex >= 0 && m.ScheduleIndex < len(m.ScheduleTimerNames)
	
	if isEditing {
		b.WriteString(SectionTitleStyle.Render("Confirm Schedule Update") + "\n\n")
	} else {
		b.WriteString(SectionTitleStyle.Render("Confirm Schedule") + "\n\n")
	}

	if m.ScheduleData == nil {
		b.WriteString("No schedule data available\n")
		return b.String()
	}

	// Check for duplicates (only when creating new, not editing)
	if !isEditing {
		duplicates, _ := schedule.CheckDuplicate(m.ScheduleData.Engine, m.ScheduleData.Databases)
		if len(duplicates) > 0 {
			b.WriteString(AuthStyle.Render("⚠ Warning: Backup already scheduled for:\n"))
			for _, db := range duplicates {
				b.WriteString(fmt.Sprintf("  - %s\n", db))
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("Schedule Details:\n\n")
	b.WriteString(fmt.Sprintf("Engine: %s\n", EngineNameStyle.Render(m.ScheduleData.Engine)))
	b.WriteString(fmt.Sprintf("Databases: %s\n", strings.Join(m.ScheduleData.Databases, ", ")))
	b.WriteString(fmt.Sprintf("Time: %s\n\n", m.ScheduleData.Time))

	if isEditing {
		b.WriteString(fmt.Sprintf("Update %s backup schedule to run at %s:\n", 
			m.ScheduleData.Engine, m.ScheduleData.Time))
		for _, db := range m.ScheduleData.Databases {
			b.WriteString(fmt.Sprintf("  • %s\n", db))
		}
	} else {
		b.WriteString(fmt.Sprintf("For %s, the following databases' daily backups will happen at %s:\n", 
			m.ScheduleData.Engine, m.ScheduleData.Time))
		for _, db := range m.ScheduleData.Databases {
			b.WriteString(fmt.Sprintf("  • %s\n", db))
		}
	}

	b.WriteString("\n")

	// Check for duplicates only when creating
	duplicates, _ := schedule.CheckDuplicate(m.ScheduleData.Engine, m.ScheduleData.Databases)
	if !isEditing && len(duplicates) > 0 {
		b.WriteString(AuthStyle.Render("Cannot schedule: duplicates detected\n"))
		b.WriteString(FooterStyle.Render(" ESC back    Q exit "))
	} else {
		if isEditing {
			b.WriteString(FooterStyle.Render(" Enter confirm update    ESC back to edit    Q exit "))
		} else {
			b.WriteString(FooterStyle.Render(" Enter confirm    ESC back to edit    Q exit "))
		}
	}

	return b.String()
}
