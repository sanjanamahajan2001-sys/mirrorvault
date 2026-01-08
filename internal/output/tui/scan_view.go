package tui

import (
	"fmt"
	"strings"

	"mirrorvault/pkg/model"
)

func (m TUIModel) viewScan() string {
	var b strings.Builder

	// Unified header (now shared across all views)
	renderHeader(&b, m.Mode)

	// Show scan-only message when in ScanMode
	if m.Mode == ScanMode {
		b.WriteString(NoAuthStyle.Render("ℹ This is scan-only mode. To create backups, run: sudo mirrorvault backup") + "\n\n")
	}

	renderSection(&b, "SQL DATABASES", model.SQL, m.ScanResult)
	renderDivider(&b)
	renderSection(&b, "NOSQL DATABASES", model.NoSQL, m.ScanResult)

	qHint := KeyHintStyle.Render("Q")

	var footerContent string
	if m.Mode == ScheduleMode {
		enterHint := KeyHintStyle.Render("ENTER")
		footerContent = fmt.Sprintf(" %s proceed to schedule-daily backup    %s exit ", enterHint, qHint)
	} else if m.Mode == ScanMode {
		// Scan mode: only show exit option
		footerContent = fmt.Sprintf(" %s exit ", qHint)
	} else {
		enterHint := KeyHintStyle.Render("ENTER")
		footerContent = fmt.Sprintf(" %s proceed to backup    %s exit ", enterHint, qHint)
	}
	b.WriteString("\n" + FooterStyle.Render(footerContent))

	return b.String()
}
