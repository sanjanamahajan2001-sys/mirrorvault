package tui

import (
	"fmt"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m TUIModel) updateScheduleTime(msg tea.Msg) (TUIModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			// Validate time format
			time := strings.TrimSpace(m.ScheduleTime)
			if time == "" {
				return m, nil
			}

			// Normalize time format
			normalizedTime, err := normalizeTime(time)
			if err != nil {
				// Invalid time, don't proceed
				return m, nil
			}

			m.ScheduleTime = normalizedTime

			// Update schedule data with normalized time
			if m.ScheduleData != nil {
				m.ScheduleData.Time = normalizedTime
			}

			// Move to confirmation view
			m.ViewState = ViewScheduleConfirm
			return m, nil

		case "backspace":
			if len(m.ScheduleTime) > 0 {
				m.ScheduleTime = m.ScheduleTime[:len(m.ScheduleTime)-1]
			}

		case "esc":
			// Go back - check if we're editing from list view, duplicate view, or creating new
			if len(m.DuplicateTimerNames) > 0 {
				// Editing from duplicate view - go back to duplicate view
				m.ScheduleTime = ""
				m.ViewState = ViewScheduleDuplicate
			} else if len(m.ScheduleTimerNames) > 0 {
				// Editing from list view - go back to list
				m.ScheduleTime = ""
				m.ViewState = ViewScheduleList
			} else {
				// Creating new - go back to DB selection
				m.ScheduleTime = ""
				m.ViewState = ViewSelectDB
			}
			return m, nil

		case "q", "ctrl+c":
			m.Exit = true
			return m, tea.Quit

		default:
			// Allow only digits, colon, and space
			if len(msg.Runes) > 0 {
				r := msg.Runes[0]
				if (r >= '0' && r <= '9') || r == ':' || r == ' ' {
					m.ScheduleTime += string(r)
				}
			}
		}
	}

	return m, nil
}

func (m TUIModel) viewScheduleTime() string {
	var b strings.Builder

	renderHeader(&b, m.Mode)

	// Check if we're editing an existing schedule or creating a new one
	isEditing := m.ScheduleData != nil && m.ViewState == ViewScheduleTime && len(m.Schedules) > 0

	if isEditing {
		b.WriteString(SectionTitleStyle.Render("Edit Backup Time") + "\n\n")
		b.WriteString(fmt.Sprintf("Engine: %s\n", EngineNameStyle.Render(m.ScheduleData.Engine)))
		b.WriteString(fmt.Sprintf("Databases: %s\n", strings.Join(m.ScheduleData.Databases, ", ")))
		b.WriteString(fmt.Sprintf("Current Time: %s\n\n", m.ScheduleData.Time))
	} else {
		b.WriteString(SectionTitleStyle.Render("Enter backup time") + "\n\n")

		// Get current engine and selected databases
		currentEngine := m.currentEngine()
		if currentEngine == nil {
			return b.String()
		}

		// Get selected databases for current engine
		selectedDBs := []string{}
		if dbMap, ok := m.Selection.SelectedDBs[currentEngine.Engine]; ok {
			for dbName, selected := range dbMap {
				if selected {
					selectedDBs = append(selectedDBs, dbName)
				}
			}
		}

		b.WriteString(fmt.Sprintf("Engine: %s\n", EngineNameStyle.Render(currentEngine.Engine)))
		b.WriteString(fmt.Sprintf("Databases: %s\n\n", strings.Join(selectedDBs, ", ")))
	}

	b.WriteString("Enter time in 00:00 to 24:00 format\n")
	b.WriteString("Examples: 6, 6:30, 14:00, 23:45\n\n")

	// Show current input
	inputDisplay := m.ScheduleTime
	if inputDisplay == "" {
		inputDisplay = "_"
	}
	b.WriteString(ItemStyle.Render(fmt.Sprintf("Time: %s", inputDisplay)) + "\n\n")

	b.WriteString(FooterStyle.Render(" Enter confirm    ESC back    Ctrl+C exit "))

	return b.String()
}

// normalizeTime normalizes time input to HH:MM format
// Accepts: "6", "6:30", "14:00", "23:45"
// Returns: "06:00", "06:30", "14:00", "23:45"
func normalizeTime(input string) (string, error) {
	input = strings.TrimSpace(input)

	// Remove any spaces
	input = strings.ReplaceAll(input, " ", "")

	// Pattern 1: Just hour (e.g., "6", "14")
	hourOnly := regexp.MustCompile(`^(\d{1,2})$`)
	if matches := hourOnly.FindStringSubmatch(input); matches != nil {
		hour := matches[1]
		if len(hour) == 1 {
			hour = "0" + hour
		}
		hourNum := 0
		fmt.Sscanf(hour, "%d", &hourNum)
		if hourNum < 0 || hourNum > 24 {
			return "", fmt.Errorf("hour must be between 0 and 24")
		}
		if hourNum == 24 {
			return "23:59", nil // 24:00 is not valid, use 23:59
		}
		return fmt.Sprintf("%s:00", hour), nil
	}

	// Pattern 2: HH:MM (e.g., "6:30", "14:00", "23:45")
	timePattern := regexp.MustCompile(`^(\d{1,2}):(\d{1,2})$`)
	if matches := timePattern.FindStringSubmatch(input); matches != nil {
		hour := matches[1]
		minute := matches[2]

		// Pad hour if needed
		if len(hour) == 1 {
			hour = "0" + hour
		}
		// Pad minute if needed
		if len(minute) == 1 {
			minute = "0" + minute
		}

		hourNum := 0
		minuteNum := 0
		fmt.Sscanf(hour, "%d", &hourNum)
		fmt.Sscanf(minute, "%d", &minuteNum)

		if hourNum < 0 || hourNum > 23 {
			return "", fmt.Errorf("hour must be between 0 and 23")
		}
		if minuteNum < 0 || minuteNum > 59 {
			return "", fmt.Errorf("minute must be between 0 and 59")
		}

		return fmt.Sprintf("%s:%s", hour, minute), nil
	}

	return "", fmt.Errorf("invalid time format")
}
