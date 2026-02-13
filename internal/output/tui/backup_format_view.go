package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m TUIModel) updateBackupFormatSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up":
		if m.BackupFormatIndex > 0 {
			m.BackupFormatIndex--
		}
	case "down":
		if m.BackupFormatIndex < 1 {
			m.BackupFormatIndex++
		}
	case "enter":
		if m.BackupFormatIndex == 0 {
			m.BackupCompression = ""
		} else {
			m.BackupCompression = "gz"
		}
		return m.startBackupExecution()
	case "esc":
		m.ViewState = ViewSelectDB
		return m, nil
	}
	return m, nil
}

func (m TUIModel) viewBackupFormatSelect() string {
	var b strings.Builder
	b.WriteString(SectionTitleStyle.Render("Select Backup Format") + "\n\n")

	options := []string{
		"Native format (no compression)",
		"Compressed (gzip)",
	}
	for i, opt := range options {
		cursor := "  "
		if i == m.BackupFormatIndex {
			cursor = "> "
		}
		b.WriteString(fmt.Sprintf("%s%s\n", cursor, opt))
	}

	b.WriteString("\n" + FooterStyle.Render(" ↑/↓ move • Enter select • Esc back "))
	return b.String()
}
