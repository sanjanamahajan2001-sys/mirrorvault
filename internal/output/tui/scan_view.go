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

	renderSection(&b, "SQL DATABASES", model.SQL, m.ScanResult)
	renderDivider(&b)
	renderSection(&b, "NOSQL DATABASES", model.NoSQL, m.ScanResult)

	enterHint := KeyHintStyle.Render("ENTER")
	qHint := KeyHintStyle.Render("Q")

	footerContent := fmt.Sprintf(" %s proceed to backup    %s exit ", enterHint, qHint)
	b.WriteString("\n" + FooterStyle.Render(footerContent))

	return b.String()
}
