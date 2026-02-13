package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// formatBytes converts bytes to human-readable format
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func (m TUIModel) viewExecute() string {
	var b strings.Builder

	// 1. Start with the title
	b.WriteString(SectionTitleStyle.Render("Executing Backups") + "\n\n")

	// Define widths for the internal columns
	// The box is 50 wide, minus 3 padding on each side = 44 usable width
	leftWidth := 30

	for i, item := range m.Exec.Items {
		if i > 0 && len(m.Exec.Items) > 1 {
			b.WriteString("\n" + DividerStyle.Render(strings.Repeat("─", 44)) + "\n\n")
		}

		status := "⏳ pending"
		switch item.Status {
		case ExecRunning:
			status = "▶ running"
		case ExecDone:
			status = "✔ done"
		case ExecFailed:
			status = "✖ failed"
		}

		// 2. Create the first line: Engine / DB | Status
		leftText := item.Engine + " / " + item.Database
		if len(leftText) > leftWidth {
			leftText = leftText[:leftWidth-3] + "..."
		}

		// Use fmt.Sprintf to ensure fixed-width padding for the left column
		// This forces "done" to align to the right side regardless of DB name length
		line := fmt.Sprintf("%-*s %s", leftWidth, ItemStyle.Render(leftText), status)
		b.WriteString(line + "\n")

		// 3. File path / Error lines
		if item.Status == ExecDone && item.Path != "" {
			fileInfo := fmt.Sprintf("   ↳ %s (%s)", item.Path, formatBytes(item.Size))
			b.WriteString(ItemStyle.Render(fileInfo) + "\n")
			b.WriteString(ItemStyle.Render("   ↳ Validation: OK") + "\n")
		}

	if item.DriveStatus != DriveNone {
		driveLine := "   ↳ Drive: "
		switch item.DriveStatus {
		case DriveChecking:
			if item.DriveAccountTotal > 0 {
				driveLine += fmt.Sprintf("checking free space (%s / %s remaining)", formatBytes(item.DriveAccountRemain), formatBytes(item.DriveAccountTotal))
			} else {
				driveLine += "checking free space"
			}
		case DriveUploading:
			driveLine += "uploading backup"
		case DriveDone:
			if item.DriveRemoteName != "" {
				driveLine += fmt.Sprintf("uploaded (%s)", item.DriveRemoteName)
			} else {
				driveLine += "uploaded"
			}
		case DriveSkipped:
			if item.DriveMessage != "" {
				driveLine += "skipped - " + item.DriveMessage
			} else {
				driveLine += "skipped"
			}
		case DriveFailed:
			if item.DriveErr != nil {
				driveLine += "failed - " + item.DriveErr.Error()
			} else {
				driveLine += "failed"
			}
		}
		b.WriteString(ItemStyle.Render(driveLine) + "\n")
	}

		if item.Status == ExecFailed && item.Err != nil {
			errorMsg := fmt.Sprintf("   ↳ Error: %s", item.Err.Error())
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(errorMsg) + "\n")
		}
	}

	if m.Exec.Done {
		b.WriteString("\n")
		hasFailures := false
		for _, item := range m.Exec.Items {
			if item.Status == ExecFailed {
				hasFailures = true
				break
			}
		}

		if hasFailures {
			b.WriteString(AuthStyle.Render("Backup completed with errors") + "\n\n")
		} else {
			b.WriteString(NoAuthStyle.Render("Backup completed successfully") + "\n\n")
		}

		b.WriteString(FooterStyle.Render(" Press Enter to exit "))
	}

	// IMPORTANT: Join everything and render the box once.
	// The "\n\n" at the start provides top margin outside the box.
	return "\n\n" + SummaryBoxStyle.Render(b.String())
}
