package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
)

func (m TUIModel) viewRestoreHistory() string {
	var b strings.Builder
	b.WriteString(SectionTitleStyle.Render("Restore History") + "\n\n")

	if len(m.RestoreHistory) == 0 {
		b.WriteString(ItemStyle.Render("No restore operations found.") + "\n\n")
		b.WriteString(FooterStyle.Render("Press Esc to go back • Ctrl+C to exit"))
		return b.String()
	}
	
	// Clamp scroll offset to valid range (don't modify model, just use clamped value)
	// Always start from the beginning (index 0 = Restore #1) if offset is invalid
	scrollOffset := m.RestoreHistoryScrollOffset
	if scrollOffset < 0 || scrollOffset >= len(m.RestoreHistory) {
		scrollOffset = 0
	}

	// Color styles
	successStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#a6e3a1")).
		Bold(true)
	failureStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#f38ba8")).
		Bold(true)
	engineStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#89b4fa")).
		Bold(true)
	dbStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#a6e3a1"))
	infoStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#cdd6f4"))
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#f9e2af")).
		Bold(true)

	// Calculate available height for scrolling
	// Reserve space: 2 lines for title, 3-4 lines for footer/scroll indicator = 6-7 lines
	// Be more conservative to ensure first item is fully visible
	availableHeight := m.TerminalHeight - 7
	if availableHeight < 8 {
		availableHeight = 8 // Minimum to show at least part of one restore entry
	}

	// Determine which items to show based on scroll offset
	// Always ensure we start from a valid position (index 0 = Restore #1)
	startIdx := scrollOffset
	if startIdx < 0 {
		startIdx = 0
	}
	if startIdx >= len(m.RestoreHistory) {
		startIdx = 0
	}
	
	// Estimate lines per restore: base 8-10 lines, more if error exists (error can wrap)
	// Be conservative to ensure items fit on screen
	estimatedLinesPerRestore := 12 // More conservative estimate to account for wrapped errors
	maxItems := availableHeight / estimatedLinesPerRestore
	if maxItems < 1 {
		maxItems = 1
	}
	
	endIdx := startIdx + maxItems
	if endIdx > len(m.RestoreHistory) {
		endIdx = len(m.RestoreHistory)
	}
	
	// Ensure we show at least one item if history exists
	if len(m.RestoreHistory) > 0 && endIdx <= startIdx {
		endIdx = startIdx + 1
		if endIdx > len(m.RestoreHistory) {
			endIdx = len(m.RestoreHistory)
		}
	}

	// Show items
	for i := startIdx; i < endIdx; i++ {
		history := m.RestoreHistory[i]
		
		// Visual separator between restorations
		if i > startIdx {
			b.WriteString("\n" + DividerStyle.Render(strings.Repeat("═", 70)) + "\n\n")
		}

		// Status indicator
		statusText := successStyle.Render("✓ SUCCESS")
		if !history.Success {
			statusText = failureStyle.Render("✗ FAILED")
		}
		if history.RolledBack {
			statusText += " " + lipgloss.NewStyle().Foreground(lipgloss.Color("#fab387")).Bold(true).Render("(ROLLED BACK)")
		}

		// Main info box
		// Border width: 70 chars total
		// Content line format: │ %s │ = 2 (left border+space) + content + 2 (space+right border) = 70
		// So content width = 70 - 2 - 2 = 66
		infoBoxWidth := 70
		infoContentWidth := infoBoxWidth - 4 // 66 chars (70 - 2 for left border - 2 for right border)
		
		topBorder := "┌" + strings.Repeat("─", infoBoxWidth-2) + "┐"
		bottomBorder := "└" + strings.Repeat("─", infoBoxWidth-2) + "┘"
		
		b.WriteString(topBorder + "\n")
		b.WriteString(fmt.Sprintf("│ %s │\n", padString(headerStyle.Render(fmt.Sprintf("Restore #%d - %s", i+1, history.Timestamp)), infoContentWidth)))
		b.WriteString("├" + strings.Repeat("─", infoBoxWidth-2) + "┤\n")
		
		// Engine and Database
		engineText := engineStyle.Render(history.Engine)
		dbText := dbStyle.Render(history.Database)
		b.WriteString(fmt.Sprintf("│ %s │\n", padString(fmt.Sprintf("Engine: %s", engineText), infoContentWidth)))
		b.WriteString(fmt.Sprintf("│ %s │\n", padString(fmt.Sprintf("Database: %s", dbText), infoContentWidth)))
		b.WriteString("├" + strings.Repeat("─", infoBoxWidth-2) + "┤\n")
		
		// Status
		b.WriteString(fmt.Sprintf("│ %s │\n", padString(fmt.Sprintf("Status: %s", statusText), infoContentWidth)))
		b.WriteString("├" + strings.Repeat("─", infoBoxWidth-2) + "┤\n")
		
		// Dump information
		// "Dump Path: " = 11 chars, available for path = 66 - 11 = 55 chars
		dumpPathLabel := "Dump Path: "
		dumpPathMaxLen := infoContentWidth - len(dumpPathLabel)
		dumpPath := history.DumpPath
		if len(dumpPath) > dumpPathMaxLen {
			dumpPath = "..." + dumpPath[len(dumpPath)-(dumpPathMaxLen-3):]
		}
		b.WriteString(fmt.Sprintf("│ %s │\n", padString(fmt.Sprintf("%s%s", dumpPathLabel, infoStyle.Render(dumpPath)), infoContentWidth)))
		
		formatInfo := history.DumpFormat
		if history.Compressed {
			formatInfo += " (compressed)"
		}
		if history.MultiDB {
			formatInfo += " (multi-DB)"
		}
		b.WriteString(fmt.Sprintf("│ %s │\n", padString(fmt.Sprintf("Format: %s", infoStyle.Render(formatInfo)), infoContentWidth)))
		b.WriteString("├" + strings.Repeat("─", infoBoxWidth-2) + "┤\n")
		
		// Pre-restore backup - wrap to multiple lines if needed
		// "Pre-Restore Backup: " = 20 chars, available for path = 66 - 20 = 46 chars per line
		// For continuation lines, use 20 spaces to align with "Pre-Restore Backup: "
		if history.PreRestoreBackup != "" {
			backupLabel := "Pre-Restore Backup: "
			backupIndent := strings.Repeat(" ", len(backupLabel)) // 20 spaces to align
			backupPath := history.PreRestoreBackup
			availableForPath := infoContentWidth - len(backupLabel) // 46 chars per line
			
			// Wrap path to multiple lines if needed
			// Try to break at directory boundaries ("/") when possible
			if len(backupPath) <= availableForPath {
				// Path fits on one line
				b.WriteString(fmt.Sprintf("│ %s │\n", padString(backupLabel+infoStyle.Render(backupPath), infoContentWidth)))
			} else {
				// Path is too long, need to wrap
				// Split path by "/" to break at directory boundaries when possible
				pathParts := strings.Split(backupPath, "/")
				if len(pathParts) == 1 {
					// No slashes, need to break the long path manually
					// Show as much as possible on first line, rest on continuation
					firstLinePath := backupPath
					if len(firstLinePath) > availableForPath {
						firstLinePath = backupPath[:availableForPath]
					}
					b.WriteString(fmt.Sprintf("│ %s │\n", padString(backupLabel+infoStyle.Render(firstLinePath), infoContentWidth)))
					
					// Continue with remaining path
					remaining := backupPath[len(firstLinePath):]
					for len(remaining) > 0 {
						linePath := remaining
						if len(linePath) > availableForPath-3 { // Reserve 3 for "..."
							linePath = remaining[:availableForPath-3]
						}
						b.WriteString(fmt.Sprintf("│ %s │\n", padString(backupIndent+infoStyle.Render("..."+linePath), infoContentWidth)))
						remaining = remaining[len(linePath):]
					}
				} else {
					// Has slashes, break at directory boundaries
					currentLineParts := []string{}
					currentLineLen := 0
					isFirstLine := true
					
					for _, part := range pathParts {
						partLen := len(part)
						separatorLen := 0
						if len(currentLineParts) > 0 {
							separatorLen = 1 // "/" separator
						}
						
						// If single part is too long, break it
						if partLen > availableForPath-3 && len(currentLineParts) == 0 {
							// This part alone is too long, show what fits
							if !isFirstLine {
								b.WriteString(fmt.Sprintf("│ %s │\n", padString(backupIndent+infoStyle.Render("..."), infoContentWidth)))
							}
							// Break the long part into chunks
							remaining := part
							for len(remaining) > 0 {
								chunkLen := availableForPath
								if !isFirstLine {
									chunkLen = availableForPath - 3 // Reserve 3 for "..."
								}
								if len(remaining) > chunkLen {
									chunk := remaining[:chunkLen]
									if isFirstLine {
										b.WriteString(fmt.Sprintf("│ %s │\n", padString(backupLabel+infoStyle.Render(chunk), infoContentWidth)))
										isFirstLine = false
									} else {
										b.WriteString(fmt.Sprintf("│ %s │\n", padString(backupIndent+infoStyle.Render("..."+chunk), infoContentWidth)))
									}
									remaining = remaining[chunkLen:]
								} else {
									if isFirstLine {
										b.WriteString(fmt.Sprintf("│ %s │\n", padString(backupLabel+infoStyle.Render(remaining), infoContentWidth)))
										isFirstLine = false
									} else {
										b.WriteString(fmt.Sprintf("│ %s │\n", padString(backupIndent+infoStyle.Render("..."+remaining), infoContentWidth)))
									}
									remaining = ""
								}
							}
							continue
						}
						
						// Check if adding this part would exceed available space
						if currentLineLen+separatorLen+partLen > availableForPath && len(currentLineParts) > 0 {
							// Write current line and start new one
							pathText := strings.Join(currentLineParts, "/")
							if isFirstLine {
								b.WriteString(fmt.Sprintf("│ %s │\n", padString(backupLabel+infoStyle.Render(pathText), infoContentWidth)))
								isFirstLine = false
							} else {
								// For continuation, add "..." to show it's a continuation
								b.WriteString(fmt.Sprintf("│ %s │\n", padString(backupIndent+infoStyle.Render("..."+pathText), infoContentWidth)))
							}
							currentLineParts = []string{part}
							currentLineLen = partLen
						} else {
							// Add part to current line
							currentLineParts = append(currentLineParts, part)
							currentLineLen += separatorLen + partLen
						}
					}
					
					// Write the last line
					if len(currentLineParts) > 0 {
						pathText := strings.Join(currentLineParts, "/")
						if isFirstLine {
							b.WriteString(fmt.Sprintf("│ %s │\n", padString(backupLabel+infoStyle.Render(pathText), infoContentWidth)))
						} else {
							b.WriteString(fmt.Sprintf("│ %s │\n", padString(backupIndent+infoStyle.Render("..."+pathText), infoContentWidth)))
						}
					}
				}
			}
		}
		
		// Error if failed - wrap to multiple lines if needed
		// "Error: " = 7 chars, available for error message = 66 - 7 = 59 chars per line
		// For continuation lines, use 7 spaces to align with "Error: "
		if !history.Success && history.Error != "" {
			errorLabel := "Error: "
			errorIndent := "       " // 7 spaces to align with "Error: "
			errorMsg := history.Error
			availableForError := infoContentWidth - len(errorLabel) // 59 chars per line
			
			b.WriteString("├" + strings.Repeat("─", infoBoxWidth-2) + "┤\n")
			
			// Wrap error message to multiple lines
			// Split into words and build lines that fit
			words := strings.Fields(errorMsg)
			if len(words) == 0 {
				words = []string{errorMsg} // If no spaces, treat entire message as one word
			}
			
			currentLineWords := []string{}
			currentLineLen := 0
			isFirstLine := true
			
			for _, word := range words {
				wordLen := len(word)
				spaceLen := 0
				if len(currentLineWords) > 0 {
					spaceLen = 1 // Space between words
				}
				
				// Check if adding this word would exceed available space
				if currentLineLen+spaceLen+wordLen > availableForError && len(currentLineWords) > 0 {
					// Write current line and start new one
					errorText := strings.Join(currentLineWords, " ")
					if isFirstLine {
						b.WriteString(fmt.Sprintf("│ %s │\n", padString(errorLabel+failureStyle.Render(errorText), infoContentWidth)))
						isFirstLine = false
					} else {
						b.WriteString(fmt.Sprintf("│ %s │\n", padString(errorIndent+failureStyle.Render(errorText), infoContentWidth)))
					}
					currentLineWords = []string{word}
					currentLineLen = wordLen
				} else {
					// Add word to current line
					currentLineWords = append(currentLineWords, word)
					currentLineLen += spaceLen + wordLen
				}
			}
			
			// Write the last line
			if len(currentLineWords) > 0 {
				errorText := strings.Join(currentLineWords, " ")
				if isFirstLine {
					b.WriteString(fmt.Sprintf("│ %s │\n", padString(errorLabel+failureStyle.Render(errorText), infoContentWidth)))
				} else {
					b.WriteString(fmt.Sprintf("│ %s │\n", padString(errorIndent+failureStyle.Render(errorText), infoContentWidth)))
				}
			}
		}
		
		b.WriteString(bottomBorder + "\n")
	}

	// Scroll indicator and exit
	b.WriteString("\n")
	footerText := ""
	if len(m.RestoreHistory) > endIdx-startIdx {
		scrollPercent := int(float64(scrollOffset) / float64(len(m.RestoreHistory)) * 100)
		if scrollPercent > 100 {
			scrollPercent = 100
		}
		footerText = fmt.Sprintf("[Scroll: %d%% ↑/k up ↓/j down] ", scrollPercent)
	}
	footerText += "Ctrl+C exit"
	b.WriteString(FooterStyle.Render(footerText))
	return b.String()
}

func (m TUIModel) updateRestoreHistory(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Clamp scroll offset to valid range
	if m.RestoreHistoryScrollOffset < 0 {
		m.RestoreHistoryScrollOffset = 0
	}
	if len(m.RestoreHistory) > 0 && m.RestoreHistoryScrollOffset >= len(m.RestoreHistory) {
		m.RestoreHistoryScrollOffset = 0
	}
	
	switch msg.String() {
	case "esc":
		// Go back to restore mode start
		m.ViewState = ViewRestoreSelectEngine
		m.RestoreHistoryScrollOffset = 0
		return m, nil
	case "up", "k":
		if m.RestoreHistoryScrollOffset > 0 {
			m.RestoreHistoryScrollOffset--
		}
		return m, nil
	case "down", "j":
		availableHeight := m.TerminalHeight - 7
		if availableHeight < 8 {
			availableHeight = 8
		}
		// Use same calculation as view function
		estimatedLinesPerRestore := 12
		maxItems := availableHeight / estimatedLinesPerRestore
		if maxItems < 1 {
			maxItems = 1
		}
		maxOffset := len(m.RestoreHistory) - maxItems
		if maxOffset < 0 {
			maxOffset = 0
		}
		if m.RestoreHistoryScrollOffset < maxOffset {
			m.RestoreHistoryScrollOffset++
		}
		return m, nil
	case "enter":
		// Could show detailed view in future
		return m, nil
	}
	return m, nil
}
