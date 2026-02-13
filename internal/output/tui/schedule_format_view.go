package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m TUIModel) updateScheduleFormatSelect(msg tea.KeyMsg) (TUIModel, tea.Cmd) {
	switch msg.String() {
	case "up":
		if m.ScheduleFormatIndex > 0 {
			m.ScheduleFormatIndex--
		}
	case "down":
		if m.ScheduleFormatIndex < 1 {
			m.ScheduleFormatIndex++
		}
	case "enter":
		if m.ScheduleData == nil {
			return m, nil
		}
		if m.ScheduleFormatIndex == 0 {
			m.ScheduleData.Compression = ""
		} else {
			m.ScheduleData.Compression = "gz"
		}
		m.ViewState = ViewScheduleTime
		return m, nil
	case "esc":
		m.ViewState = ViewSelectDB
		return m, nil
	}
	return m, nil
}

func (m TUIModel) viewScheduleFormatSelect() string {
	var b strings.Builder

	renderHeader(&b, m.Mode)
	b.WriteString(SectionTitleStyle.Render("Select Backup Format") + "\n\n")

	options := []string{
		"Native format (no compression)",
		"Compressed (gzip)",
	}
	for i, opt := range options {
		cursor := "  "
		if i == m.ScheduleFormatIndex {
			cursor = "> "
		}
		b.WriteString(fmt.Sprintf("%s%s\n", cursor, opt))
	}

	b.WriteString("\n" + FooterStyle.Render(" ↑/↓ move    Enter select    ESC back "))
	return b.String()
}
