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

	b.WriteString(SectionTitleStyle.Render("Executing Backups\n\n"))

	totalWidth := 44 // Total width for alignment (within the 50-width box minus padding)
	leftWidth := 30  // Width for Engine / Database
	rightWidth := 14 // Width for status

	for i, item := range m.Exec.Items {
		// Add separator between entries if there are multiple databases
		if i > 0 && len(m.Exec.Items) > 1 {
			b.WriteString("\n" + DividerStyle.Render(strings.Repeat("─", totalWidth)) + "\n\n")
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

		// Create properly aligned layout using fixed widths
		leftText := item.Engine + " / " + item.Database
		// Truncate if too long to maintain alignment
		if len(leftText) > leftWidth {
			leftText = leftText[:leftWidth-3] + "..."
		}
		
		// Apply width constraint first, then style - this ensures proper alignment
		// Combine ItemStyle with width in one style chain so lipgloss handles ANSI codes correctly
		leftStyled := ItemStyle.
			Width(leftWidth).
			Render(leftText)
		
		rightStyled := lipgloss.NewStyle().
			Width(rightWidth).
			Align(lipgloss.Right).
			Render(status)
		
		// Join them horizontally - this ensures consistent alignment
		line := lipgloss.JoinHorizontal(lipgloss.Left, leftStyled, rightStyled)
		b.WriteString(line + "\n")

		if item.Status == ExecDone && item.Path != "" {
			// Align file path with proper indentation (matching left column)
			fileInfo := fmt.Sprintf("   ↳ %s (%s)", item.Path, formatBytes(item.Size))
			b.WriteString(ItemStyle.Render(fileInfo) + "\n")
		}
		
		if item.Status == ExecFailed && item.Err != nil {
			// Display error message with proper indentation
			errorMsg := fmt.Sprintf("   ↳ Error: %s", item.Err.Error())
			// Truncate long error messages to fit the display
			if len(errorMsg) > totalWidth+10 {
				errorMsg = errorMsg[:totalWidth+7] + "..."
			}
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(errorMsg) + "\n")
		}
	}

	if m.Exec.Done {
		b.WriteString("\n")
		
		// Check if any backup failed
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

	return "\n\n" + SummaryBoxStyle.Render(b.String())
}
