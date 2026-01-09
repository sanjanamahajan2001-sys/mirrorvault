package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"mirrorvault/internal/restore"
	"mirrorvault/internal/restore/analyze"
	"mirrorvault/internal/restore/validate"
)

// padString pads a string to a specific width, accounting for ANSI color codes
func padString(s string, width int) string {
	actualWidth := lipgloss.Width(s)
	if actualWidth >= width {
		return s
	}
	return s + strings.Repeat(" ", width-actualWidth)
}

func (m TUIModel) viewRestoreSelectEngine() string {
	var b strings.Builder
	b.WriteString(SectionTitleStyle.Render("Select Database Engine for Restore") + "\n\n")

	for i, db := range m.ScanResult.Databases {
		cursor := "  "
		if i == m.Selection.EngineIndex {
			cursor = "> "
		}

		status := "●"
		if !db.Running {
			status = "○"
		}

		auth := ""
		if db.RequiresAuth {
			auth = AuthStyle.Render(" [Auth]")
		}

		b.WriteString(fmt.Sprintf("%s%s %s (%s)%s\n", cursor, status, db.Engine, db.DisplayVersion(), auth))
	}

	b.WriteString("\n" + FooterStyle.Render(" ↑/↓ move • Enter select • Q exit "))
	return b.String()
}

func (m TUIModel) viewRestoreSelectDB() string {
	engine := m.currentEngine()
	if engine == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString(SectionTitleStyle.Render(fmt.Sprintf("Select Database to Restore (%s)", engine.Engine)) + "\n\n")

	displayNames := filterDefaultDatabases(engine.Engine, engine.Names)

	for i, name := range displayNames {
		cursor := "  "
		if i == m.Selection.DBIndex {
			cursor = "> "
		}

		b.WriteString(fmt.Sprintf("%s%s\n", cursor, name))
	}

	b.WriteString("\n" + FooterStyle.Render(" ↑/↓ move • Enter select • Esc back • Q exit "))
	return b.String()
}

func (m TUIModel) viewRestoreDumpPath() string {
	var b strings.Builder
	b.WriteString(SectionTitleStyle.Render("Enter Dump File Path") + "\n\n")

	engine := m.currentEngine()
	if engine == nil {
		return ""
	}

	selectedDB := ""
	displayNames := filterDefaultDatabases(engine.Engine, engine.Names)
	if m.Selection.DBIndex >= 0 && m.Selection.DBIndex < len(displayNames) {
		selectedDB = displayNames[m.Selection.DBIndex]
	}

	b.WriteString(fmt.Sprintf("Engine: %s\n", engine.Engine))
	b.WriteString(fmt.Sprintf("Database: %s\n\n", selectedDB))
	
	b.WriteString(NoAuthStyle.Render("Enter the full path to the dump file on this server:") + "\n\n")
	
	if m.RestoreDumpPath == "" {
		b.WriteString(ItemStyle.Render("  [Type or paste dump file path here]") + "\n")
	} else {
		b.WriteString(ItemStyle.Render(fmt.Sprintf("  %s", m.RestoreDumpPath)) + "\n")
	}

	// Show error message if validation failed
	if m.RestoreError != nil {
		errorStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f38ba8")).
			Bold(true)
		b.WriteString("\n" + errorStyle.Render("⚠ Error: "+m.RestoreError.Error()) + "\n\n")
	}

	b.WriteString("\n")
	b.WriteString(NoAuthStyle.Render("Examples:") + "\n")
	b.WriteString("  • /home/user/backups/app_db_2026-01-09.sql\n")
	b.WriteString("  • /var/backups/mirrorvault/mysql/app_db_2026-01-09.sql\n")
	b.WriteString("  • /tmp/dump.sql.gz\n")
	b.WriteString("  • ./backup.sql\n\n")

	b.WriteString(NoAuthStyle.Render("Tip: Press F1 to automatically use the latest backup") + "\n")
	b.WriteString("\n" + FooterStyle.Render(" Type path • F1 latest backup • Enter confirm • Esc back • Ctrl+C exit "))
	b.WriteString("\n" + lipgloss.NewStyle().
		Foreground(lipgloss.Color("#89b4fa")).
		Italic(true).
		Render("💡 Tip: Run 'sudo mirrorvault restore-history' to view previous restore operations"))
	return b.String()
}

func (m TUIModel) viewRestoreConfirm() string {
	var b strings.Builder
	b.WriteString(SectionTitleStyle.Render("Confirm Restore Operation") + "\n\n")

	if m.RestorePlan == nil {
		return "Error: Restore plan not initialized"
	}

	engine := m.currentEngine()
	if engine == nil {
		return "Error: Engine not selected"
	}

	b.WriteString(fmt.Sprintf("Engine: %s\n", m.RestorePlan.Engine))
	b.WriteString(fmt.Sprintf("Database: %s\n", m.RestorePlan.Database))
	b.WriteString(fmt.Sprintf("Dump Path: %s\n\n", m.RestorePlan.DumpPath))

	// Show pre-restore stats if available
	if m.PreRestoreStats != nil {
		b.WriteString("Current Database Statistics:\n")
		b.WriteString(fmt.Sprintf("  Tables: %d\n", m.PreRestoreStats.TableCount))
		b.WriteString(fmt.Sprintf("  Total Rows: %d\n", m.PreRestoreStats.TotalRows))
		b.WriteString(fmt.Sprintf("  Size: %s\n\n", formatBytes(m.PreRestoreStats.Size)))
	}

	b.WriteString(NoAuthStyle.Render("⚠ WARNING: This will replace the current database!") + "\n")
	b.WriteString(NoAuthStyle.Render("A backup will be created before restore.") + "\n\n")

	b.WriteString(FooterStyle.Render(" Enter confirm • Esc back • Q exit "))
	return b.String()
}

func (m TUIModel) viewRestoreProgress() string {
	var b strings.Builder
	b.WriteString(SectionTitleStyle.Render("Restoring Database") + "\n\n")

	if m.RestorePlan == nil {
		return "Error: Restore plan not initialized"
	}

	// Better formatted info section
	engineStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#89b4fa"))
	dbStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#a6e3a1"))
	
	engineText := engineStyle.Render(m.RestorePlan.Engine)
	dbText := dbStyle.Render(m.RestorePlan.Database)
	
	dumpPath := m.RestorePlan.DumpPath
	if len(dumpPath) > 45 {
		dumpPath = "..." + dumpPath[len(dumpPath)-42:]
	}
	dumpText := ItemStyle.Render(dumpPath)
	
	// Fixed width for info box: 63 chars total, "│ Engine:   " = 11 chars, " │" = 2 chars, so content = 50 chars
	infoContentWidth := 50
	
	b.WriteString("┌───────────────────────────────────────────────────────────────┐\n")
	b.WriteString(fmt.Sprintf("│ Engine:   %s │\n", padString(engineText, infoContentWidth)))
	b.WriteString(fmt.Sprintf("│ Database: %s │\n", padString(dbText, infoContentWidth)))
	b.WriteString(fmt.Sprintf("│ Dump:     %s │\n", padString(dumpText, infoContentWidth)))
	b.WriteString("└───────────────────────────────────────────────────────────────┘\n\n")

	// Progress bar with better formatting
	progress := m.RestoreProgress
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}

	barWidth := 50
	filled := int(progress * float64(barWidth))
	empty := barWidth - filled

	// Use colored progress bar
	filledBar := lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1")).Render(strings.Repeat("█", filled))
	emptyBar := lipgloss.NewStyle().Foreground(lipgloss.Color("#585b70")).Render(strings.Repeat("░", empty))
	bar := filledBar + emptyBar
	percentage := int(progress * 100)

	b.WriteString("Progress: " + bar + fmt.Sprintf(" %d%%\n\n", percentage))

	// Current step with better formatting
	if m.RestoreStep != "" {
		stepStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f9e2af")).
			Bold(true)
		b.WriteString("Step: " + stepStyle.Render(m.RestoreStep) + "\n")
	}

	// Message with better formatting
	if m.RestoreMessage != "" {
		msgStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#bac2de"))
		b.WriteString("Status: " + msgStyle.Render(m.RestoreMessage) + "\n")
	}

	// Error
	if m.RestoreError != nil {
		b.WriteString("\n")
		errorMsg := fmt.Sprintf("Error: %s", m.RestoreError.Error())
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(errorMsg) + "\n")
	}

	// Pre-restore backup path with better formatting
	if m.RestoreBackupPath != "" {
		b.WriteString("\n")
		backupStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#89b4fa")).
			Italic(true)
		b.WriteString("Pre-restore backup: " + backupStyle.Render(m.RestoreBackupPath) + "\n")
	}

	// Detailed before/after comparison with color coding
	if m.PostRestoreStats != nil && m.RestoreError == nil {
		b.WriteString("\n")
		b.WriteString(SectionTitleStyle.Render("═══════════════════════════════════════════════════════════") + "\n")
		b.WriteString(SectionTitleStyle.Render("Restore Summary") + "\n")
		b.WriteString(SectionTitleStyle.Render("═══════════════════════════════════════════════════════════") + "\n\n")
		
		// Color styles for before/after
		beforeStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f38ba8")).
			Bold(true)
		afterStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#a6e3a1")).
			Bold(true)
		metricStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#cdd6f4"))
		
		// Summary comparison table with better formatting
		// Column widths: Metric=24, BEFORE=16, AFTER=16
		metricHeader := metricStyle.Render("Metric")
		beforeHeader := beforeStyle.Render("BEFORE")
		afterHeader := afterStyle.Render("AFTER")
		
		// Build borders with exact widths: 24, 16, 16
		topBorder := "┌" + strings.Repeat("─", 24+2) + "┬" + strings.Repeat("─", 16+2) + "┬" + strings.Repeat("─", 16+2) + "┐"
		midBorder := "├" + strings.Repeat("─", 24+2) + "┼" + strings.Repeat("─", 16+2) + "┼" + strings.Repeat("─", 16+2) + "┤"
		bottomBorder := "└" + strings.Repeat("─", 24+2) + "┴" + strings.Repeat("─", 16+2) + "┴" + strings.Repeat("─", 16+2) + "┘"
		
		b.WriteString(topBorder + "\n")
		b.WriteString(fmt.Sprintf("│ %s │ %s │ %s │\n", 
			padString(metricHeader, 24), padString(beforeHeader, 16), padString(afterHeader, 16)))
		b.WriteString(midBorder + "\n")
		
		preTables := 0
		preRows := int64(0)
		preSize := int64(0)
		if m.PreRestoreStats != nil {
			preTables = m.PreRestoreStats.TableCount
			preRows = m.PreRestoreStats.TotalRows
			preSize = m.PreRestoreStats.Size
		}
		
		// Format with colors
		beforeTables := beforeStyle.Render(fmt.Sprintf("%d", preTables))
		afterTables := afterStyle.Render(fmt.Sprintf("%d", m.PostRestoreStats.TableCount))
		b.WriteString(fmt.Sprintf("│ %s │ %s │ %s │\n", 
			padString(metricStyle.Render("Tables"), 24), 
			padString(beforeTables, 16), 
			padString(afterTables, 16)))
		
		beforeRows := beforeStyle.Render(fmt.Sprintf("%d", preRows))
		afterRows := afterStyle.Render(fmt.Sprintf("%d", m.PostRestoreStats.TotalRows))
		b.WriteString(fmt.Sprintf("│ %s │ %s │ %s │\n", 
			padString(metricStyle.Render("Total Rows"), 24), 
			padString(beforeRows, 16), 
			padString(afterRows, 16)))
		
		beforeSize := beforeStyle.Render(formatBytes(preSize))
		afterSize := afterStyle.Render(formatBytes(m.PostRestoreStats.Size))
		b.WriteString(fmt.Sprintf("│ %s │ %s │ %s │\n", 
			padString(metricStyle.Render("Database Size"), 24), 
			padString(beforeSize, 16), 
			padString(afterSize, 16)))
		b.WriteString(bottomBorder + "\n\n")

		// Detailed table comparison with better formatting
		if len(m.PostRestoreStats.Tables) > 0 || (m.PreRestoreStats != nil && len(m.PreRestoreStats.Tables) > 0) {
			b.WriteString(SectionTitleStyle.Render("═══════════════════════════════════════════════════════════") + "\n")
			b.WriteString(SectionTitleStyle.Render("Table Details") + "\n")
			b.WriteString(SectionTitleStyle.Render("═══════════════════════════════════════════════════════════") + "\n\n")
			
			// Color styles
			beforeStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#f38ba8")).
				Bold(true)
			afterStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#a6e3a1")).
				Bold(true)
			tableNameStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#89b4fa")).
				Bold(true).
				Underline(true)
			propertyStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#cdd6f4"))
			
			// Show each table with before/after comparison
			for i, postTable := range m.PostRestoreStats.Tables {
				if i > 0 {
					b.WriteString("\n" + DividerStyle.Render(strings.Repeat("─", 70)) + "\n\n")
				}
				
				// Find corresponding pre-restore table
				var preTable *analyze.TableStats
				if m.PreRestoreStats != nil {
					for _, t := range m.PreRestoreStats.Tables {
						if t.Name == postTable.Name {
							preTable = &t
							break
						}
					}
				}
				
				tableNameText := tableNameStyle.Render(postTable.Name)
				propertyHeader := propertyStyle.Render("Property")
				beforeHeader := beforeStyle.Render("BEFORE")
				afterHeader := afterStyle.Render("AFTER")
				
				// Table header width: 66 chars total, "│ Table: " = 8 chars, " │" = 2 chars, so content = 56 chars
				tableHeaderContentWidth := 56
				tableNamePadded := padString(tableNameText, tableHeaderContentWidth)
				tableHeaderBorder := "┌" + strings.Repeat("─", tableHeaderContentWidth+2) + "┐"
				b.WriteString(tableHeaderBorder + "\n")
				b.WriteString(fmt.Sprintf("│ Table: %s │\n", tableNamePadded))
				
				// Property table borders: 24, 16, 16
				propTopBorder := "├" + strings.Repeat("─", 24+2) + "┬" + strings.Repeat("─", 16+2) + "┬" + strings.Repeat("─", 16+2) + "┤"
				propMidBorder := "├" + strings.Repeat("─", 24+2) + "┼" + strings.Repeat("─", 16+2) + "┼" + strings.Repeat("─", 16+2) + "┤"
				propBottomBorder := "└" + strings.Repeat("─", 24+2) + "┴" + strings.Repeat("─", 16+2) + "┴" + strings.Repeat("─", 16+2) + "┘"
				
				b.WriteString(propTopBorder + "\n")
				b.WriteString(fmt.Sprintf("│ %s │ %s │ %s │\n", 
					padString(propertyHeader, 24), 
					padString(beforeHeader, 16), 
					padString(afterHeader, 16)))
				b.WriteString(propMidBorder + "\n")
				
				preRows := int64(0)
				preCols := 0
				if preTable != nil {
					preRows = preTable.RowCount
					preCols = len(preTable.Columns)
				}
				
				beforeRowsStr := beforeStyle.Render(fmt.Sprintf("%d", preRows))
				afterRowsStr := afterStyle.Render(fmt.Sprintf("%d", postTable.RowCount))
				b.WriteString(fmt.Sprintf("│ %s │ %s │ %s │\n", 
					padString(propertyStyle.Render("Rows"), 24), 
					padString(beforeRowsStr, 16), 
					padString(afterRowsStr, 16)))
				
				beforeColsStr := beforeStyle.Render(fmt.Sprintf("%d", preCols))
				afterColsStr := afterStyle.Render(fmt.Sprintf("%d", len(postTable.Columns)))
				b.WriteString(fmt.Sprintf("│ %s │ %s │ %s │\n", 
					padString(propertyStyle.Render("Columns"), 24), 
					padString(beforeColsStr, 16), 
					padString(afterColsStr, 16)))
				b.WriteString(propBottomBorder + "\n\n")
				
				// Show column information with better formatting
				if len(postTable.Columns) > 0 {
					colHeaderStyle := lipgloss.NewStyle().
						Foreground(lipgloss.Color("#f9e2af")).
						Bold(true)
					colNameStyle := lipgloss.NewStyle().
						Foreground(lipgloss.Color("#89b4fa"))
					colTypeStyle := lipgloss.NewStyle().
						Foreground(lipgloss.Color("#bac2de"))
					
					colsHeader := colHeaderStyle.Render("Columns:")
					colNameHeader := colHeaderStyle.Render("Column Name")
					colTypeHeader := colHeaderStyle.Render("Type")
					nullableHeader := colHeaderStyle.Render("Nullable")
					
					b.WriteString(colsHeader + "\n")
					// Column table borders: 24, 28, 10
					colTopBorder := "┌" + strings.Repeat("─", 24+2) + "┬" + strings.Repeat("─", 28+2) + "┬" + strings.Repeat("─", 10+2) + "┐"
					colMidBorder := "├" + strings.Repeat("─", 24+2) + "┼" + strings.Repeat("─", 28+2) + "┼" + strings.Repeat("─", 10+2) + "┤"
					colBottomBorder := "└" + strings.Repeat("─", 24+2) + "┴" + strings.Repeat("─", 28+2) + "┴" + strings.Repeat("─", 10+2) + "┘"
					
					b.WriteString(colTopBorder + "\n")
					b.WriteString(fmt.Sprintf("│ %s │ %s │ %s │\n", 
						padString(colNameHeader, 24), 
						padString(colTypeHeader, 28), 
						padString(nullableHeader, 10)))
					b.WriteString(colMidBorder + "\n")
					
					for _, col := range postTable.Columns {
						nullable := "NO"
						nullableColor := lipgloss.Color("#f38ba8")
						if col.Nullable {
							nullable = "YES"
							nullableColor = lipgloss.Color("#a6e3a1")
						}
						colName := col.Name
						if len(colName) > 24 {
							colName = colName[:21] + "..."
						}
						colType := col.Type
						if len(colType) > 28 {
							colType = colType[:25] + "..."
						}
						
						nullableStr := lipgloss.NewStyle().Foreground(nullableColor).Render(nullable)
						b.WriteString(fmt.Sprintf("│ %s │ %s │ %s │\n", 
							padString(colNameStyle.Render(colName), 24), 
							padString(colTypeStyle.Render(colType), 28), 
							padString(nullableStr, 10)))
					}
					b.WriteString(colBottomBorder + "\n\n")
				}
				
				// Show sample rows with better formatting
				if len(postTable.SampleRows) > 0 {
					sampleHeaderStyle := lipgloss.NewStyle().
						Foreground(lipgloss.Color("#f9e2af")).
						Bold(true)
					colHeaderStyle := lipgloss.NewStyle().
						Foreground(lipgloss.Color("#89b4fa")).
						Bold(true)
					cellStyle := lipgloss.NewStyle().
						Foreground(lipgloss.Color("#bac2de"))
					
					b.WriteString(sampleHeaderStyle.Render(fmt.Sprintf("Sample Rows (last %d rows):", len(postTable.SampleRows))) + "\n")
					
					// Get column names
					colNames := []string{}
					for _, col := range postTable.Columns {
						colNames = append(colNames, col.Name)
					}
					
					if len(colNames) > 0 {
						// Calculate column widths (max 20 chars)
						colWidths := make([]int, len(colNames))
						for i, colName := range colNames {
							colWidths[i] = len(colName)
							if colWidths[i] > 20 {
								colWidths[i] = 20
							}
							if colWidths[i] < 10 {
								colWidths[i] = 10
							}
						}
						
						// Build table header
						header := "│"
						separator := "├"
						totalWidth := 1
						for i, colName := range colNames {
							displayName := colName
							if len(displayName) > colWidths[i] {
								displayName = displayName[:colWidths[i]-3] + "..."
							}
							width := colWidths[i] + 2
							colHeaderText := colHeaderStyle.Render(displayName)
							header += " " + padString(colHeaderText, colWidths[i]) + " │"
							separator += strings.Repeat("─", width) + "┼"
							totalWidth += width + 1
						}
						separator = strings.TrimSuffix(separator, "┼") + "┤"
						
						topBorder := "┌"
						for i := range colNames {
							topBorder += strings.Repeat("─", colWidths[i]+2) + "┬"
						}
						topBorder = strings.TrimSuffix(topBorder, "┬") + "┐"
						
						bottomBorder := "└"
						for i := range colNames {
							bottomBorder += strings.Repeat("─", colWidths[i]+2) + "┴"
						}
						bottomBorder = strings.TrimSuffix(bottomBorder, "┴") + "┘"
						
						b.WriteString(topBorder + "\n")
						b.WriteString(header + "\n")
						b.WriteString(separator + "\n")
						
						// Show rows (limit to 10)
						rowsToShow := postTable.SampleRows
						if len(rowsToShow) > 10 {
							rowsToShow = rowsToShow[:10]
						}
						
						for _, row := range rowsToShow {
							rowStr := "│"
							for i, colName := range colNames {
								val := row[colName]
								if val == "" {
									val = lipgloss.NewStyle().Foreground(lipgloss.Color("#585b70")).Render("<NULL>")
								} else {
									if len(val) > colWidths[i] {
										val = val[:colWidths[i]-3] + "..."
									}
									val = cellStyle.Render(val)
								}
								rowStr += " " + padString(val, colWidths[i]) + " │"
							}
							b.WriteString(rowStr + "\n")
						}
						
						b.WriteString(bottomBorder + "\n\n")
					}
				}
			}
		}
	}

	if m.RestoreError == nil && m.RestoreProgress >= 1.0 {
		b.WriteString("\n")
		b.WriteString(NoAuthStyle.Render("✓ Restore completed successfully!") + "\n\n")
		// Add message about restore history command
		historyMsg := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#89b4fa")).
			Italic(true).
			Render("💡 Tip: Run 'sudo mirrorvault restore-history' to view all restore operations")
		b.WriteString(historyMsg + "\n\n")
	} else if m.RestoreError != nil {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("✗ Restore failed") + "\n")
		if m.RestoreBackupPath != "" {
			b.WriteString(NoAuthStyle.Render("Database has been rolled back to previous state.") + "\n")
		}
		// Add message about restore history command
		historyMsg := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#89b4fa")).
			Italic(true).
			Render("💡 Tip: Run 'sudo mirrorvault restore-history' to view all restore operations")
		b.WriteString("\n" + historyMsg + "\n\n")
	}

	fullContent := b.String()
	
	// Split content into lines for scrolling
	lines := strings.Split(fullContent, "\n")
	
	// Calculate available height (reserve 3 lines for footer/scroll indicators)
	availableHeight := m.TerminalHeight - 3
	if availableHeight < 10 {
		availableHeight = 10 // Minimum height
	}
	
	// Calculate max scroll offset
	maxScroll := len(lines) - availableHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	
	// Clamp scroll offset
	if m.RestoreScrollOffset > maxScroll {
		m.RestoreScrollOffset = maxScroll
	}
	if m.RestoreScrollOffset < 0 {
		m.RestoreScrollOffset = 0
	}
	
	// Get visible lines
	startLine := m.RestoreScrollOffset
	endLine := startLine + availableHeight
	if endLine > len(lines) {
		endLine = len(lines)
	}
	
	visibleLines := lines[startLine:endLine]
	result := strings.Join(visibleLines, "\n")
	
	// Add scroll indicators and footer
	var footer strings.Builder
	if maxScroll > 0 {
		scrollPercent := int(float64(m.RestoreScrollOffset) / float64(maxScroll) * 100)
		footer.WriteString(fmt.Sprintf("\n[Scroll: %d%%", scrollPercent))
		if m.RestoreScrollOffset > 0 {
			footer.WriteString(" ↑/k up")
		}
		if m.RestoreScrollOffset < maxScroll {
			footer.WriteString(" ↓/j down")
		}
		footer.WriteString("] ")
	}
	
	if m.RestoreProgress >= 1.0 || m.RestoreError != nil {
		footer.WriteString("Enter exit")
	}
	
	if footer.Len() > 0 {
		result += "\n" + FooterStyle.Render(footer.String())
	} else {
		result += "\n" + FooterStyle.Render(" Press Enter to exit ")
	}
	
	return result
}

func (m TUIModel) updateRestoreSelectEngine(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up":
		if m.Selection.EngineIndex > 0 {
			m.Selection.EngineIndex--
		}
	case "down":
		if m.Selection.EngineIndex < len(m.ScanResult.Databases)-1 {
			m.Selection.EngineIndex++
		}
	case "enter":
		engine := m.currentEngine()
		if engine == nil || len(engine.Names) == 0 {
			return m, nil
		}
		m.Selection.DBIndex = 0
		m.ViewState = ViewRestoreSelectDB
	case "q", "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m TUIModel) updateRestoreSelectDB(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	engine := m.currentEngine()
	if engine == nil {
		m.ViewState = ViewRestoreSelectEngine
		return m, nil
	}

	displayNames := filterDefaultDatabases(engine.Engine, engine.Names)

	switch msg.String() {
	case "up":
		if m.Selection.DBIndex > 0 {
			m.Selection.DBIndex--
		}
	case "down":
		if m.Selection.DBIndex < len(displayNames)-1 {
			m.Selection.DBIndex++
		}
	case "enter":
		if m.Selection.DBIndex >= 0 && m.Selection.DBIndex < len(displayNames) {
			m.RestoreDumpPath = ""
			m.ViewState = ViewRestoreDumpPath
		}
	case "esc":
		m.ViewState = ViewRestoreSelectEngine
	case "q", "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m TUIModel) updateRestoreDumpPath(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Clear any previous errors when user starts typing
	if msg.Type == tea.KeyRunes || msg.String() == "backspace" {
		m.RestoreError = nil
	}

	// Handle F1 key for latest backup
	// Check both KeyF1 type and string representation (some terminals may send it differently)
	// Also check for function key escape sequences
	keyStr := msg.String()
	if msg.Type == tea.KeyF1 || keyStr == "f1" || keyStr == "F1" {
		engine := m.currentEngine()
		if engine == nil {
			return m, nil
		}
		displayNames := filterDefaultDatabases(engine.Engine, engine.Names)
		if m.Selection.DBIndex >= 0 && m.Selection.DBIndex < len(displayNames) {
			selectedDB := displayNames[m.Selection.DBIndex]
			latestBackup, err := restore.FindLatestBackup(engine.Engine, selectedDB)
			if err != nil {
				m.RestoreError = fmt.Errorf("failed to find latest backup: %v", err)
				m.RestoreDumpPath = ""
			} else {
				m.RestoreDumpPath = latestBackup
				m.RestoreError = nil // Clear error on success
			}
		}
		return m, nil
	}

	switch msg.String() {
	case "enter":
		if m.RestoreDumpPath == "" {
			m.RestoreError = fmt.Errorf("dump file path cannot be empty")
			return m, nil
		}
		
		// Build restore plan before moving to confirmation view
		plan, err := buildRestorePlan(m)
		if err != nil {
			m.RestoreError = fmt.Errorf("failed to build restore plan: %v", err)
			return m, nil
		}
		m.RestorePlan = plan

		// Validate dump format compatibility before proceeding
		dumpInfo, err := validate.ValidateDump(plan.DumpPath)
		if err != nil {
			m.RestoreError = fmt.Errorf("dump file validation failed: %v", err)
			return m, nil
		}
		
		// Check format compatibility
		if err := validate.ValidateFormatCompatibility(dumpInfo, plan.Engine); err != nil {
			m.RestoreError = err
			return m, nil
		}

		// Clear error if validation passed
		m.RestoreError = nil

		// Analyze current database for display in confirmation
		password := ""
		if plan.RequiresAuth {
			// We'll collect password in the execution phase, but try without for stats
		}
		preStats, err := analyze.AnalyzeDatabase(plan.Engine, plan.Database, plan.RequiresAuth, password)
		if err == nil {
			m.PreRestoreStats = preStats
		}

		// Move to confirmation view
		m.ViewState = ViewRestoreConfirm
		return m, nil
	case "esc":
		m.ViewState = ViewRestoreSelectDB
	case "backspace":
		if len(m.RestoreDumpPath) > 0 {
			m.RestoreDumpPath = m.RestoreDumpPath[:len(m.RestoreDumpPath)-1]
		}
	case "ctrl+c":
		return m, tea.Quit
	case "q":
		// Only quit on 'q' if path is empty or user explicitly wants to exit
		// Allow 'q' to be typed as part of the path
		// User can use Esc to go back or Ctrl+C to quit
		if m.RestoreDumpPath == "" {
			return m, tea.Quit
		}
		// Otherwise, treat 'q' as a regular character
		fallthrough
	default:
		// Add character to path (simple input handling)
		if len(msg.Runes) > 0 {
			m.RestoreDumpPath += string(msg.Runes)
		}
	}
	return m, nil
}

func (m TUIModel) updateRestoreConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		// Plan should already be built from dump path view
		if m.RestorePlan == nil {
			// Fallback: try to build it now
			plan, err := buildRestorePlan(m)
			if err != nil {
				return m, nil
			}
			m.RestorePlan = plan
		}

		// Start restore process
		m.ViewState = ViewRestoreProgress
		m.RestoreProgress = 0.0
		startRestoreExecution(m)
		return m, restoreTick()
	case "esc":
		m.ViewState = ViewRestoreDumpPath
	case "q", "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m TUIModel) updateRestoreProgress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle scrolling when restore is complete
	if m.RestoreProgress >= 1.0 || m.RestoreError != nil {
		switch msg.String() {
		case "up", "k":
			if m.RestoreScrollOffset > 0 {
				m.RestoreScrollOffset--
			}
			return m, nil
		case "down", "j":
			m.RestoreScrollOffset++
			return m, nil
		case "pageup", "pgup":
			m.RestoreScrollOffset -= 10
			if m.RestoreScrollOffset < 0 {
				m.RestoreScrollOffset = 0
			}
			return m, nil
		case "pagedown", "pgdn":
			m.RestoreScrollOffset += 10
			return m, nil
		case "home", "g":
			m.RestoreScrollOffset = 0
			return m, nil
		case "end", "G":
			// Calculate max scroll - we'll set it in view, but set a high value here
			m.RestoreScrollOffset = 999999
			return m, nil
		case "enter", "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m TUIModel) updateRestoreProgressMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case restoreProgressMsg:
		m.RestoreStep = msg.Step
		m.RestoreProgress = msg.Progress
		m.RestoreMessage = msg.Message
		if msg.Error != nil {
			m.RestoreError = msg.Error
		}
		return m, restoreTick()
	case restoreCompleteMsg:
		m.RestoreProgress = 1.0
		if msg.Error != nil {
			m.RestoreError = msg.Error
		}
		if msg.BackupPath != "" {
			m.RestoreBackupPath = msg.BackupPath
		}
		// Set post-restore stats
		if msg.PostRestoreStats != nil {
			if stats, ok := msg.PostRestoreStats.(*analyze.DatabaseStats); ok {
				m.PostRestoreStats = stats
			}
		}
		return m, nil
	}
	return m, nil
}
