package tui

import (
	"fmt"
	"strings"

	"mirrorvault/pkg/model"

	"github.com/charmbracelet/lipgloss"
)

// ---------------- HEADER ----------------

func renderHeader(b *strings.Builder, mode Mode) {
	b.WriteString(TitleStyle.Render("🗄  MirrorVault") + "\n")
	b.WriteString(SubtitleStyle.Render("Secure Database Backup Agent") + "\n")

	if mode == ScanMode {
		b.WriteString(ModeStyle.Render("Mode: Scan (read-only)") + "\n\n")
	} else if mode == ScheduleMode {
		b.WriteString(ModeStyle.Render("Mode: Backup Scheduler") + "\n\n")
	} else {
		b.WriteString(ModeStyle.Render("Mode: Backup") + "\n\n")
	}
}

// ---------------- VERSION NORMALIZATION (UI ONLY) ----------------

func normalizeVersion(engine, raw string) string {
	raw = strings.ToLower(raw)

	switch engine {
	case "MySQL":
		return extractVersion(raw, "8.")
	case "PostgreSQL":
		return extractVersion(raw, "15.", "16.")
	case "Redis":
		return extractVersion(raw, "7.")
	default:
		return raw
	}
}

func extractVersion(s string, prefixes ...string) string {
	for _, p := range prefixes {
		if i := strings.Index(s, p); i != -1 {
			end := i
			for end < len(s) && (s[end] == '.' || (s[end] >= '0' && s[end] <= '9')) {
				end++
			}
			return s[i:end]
		}
	}
	return s
}

// ---------------- SECTIONS ----------------

func renderSection(b *strings.Builder, title string, dbType model.DatabaseType, result model.ScanResult) {
	count := 0
	for _, db := range result.Databases {
		if db.Type == dbType {
			count++
		}
	}

	b.WriteString(SectionTitleStyle.Render(fmt.Sprintf("%s (%d)", title, count)) + "\n\n")

	var tiles []string
	for _, db := range result.Databases {
		if db.Type != dbType {
			continue
		}

		authLabel := NoAuthStyle.Render("No auth")
		if db.RequiresAuth {
			authLabel = AuthStyle.Render("Auth")
		}

		displayVersion := normalizeVersion(db.Engine, db.Version)

		content := lipgloss.JoinVertical(
			lipgloss.Center,
			EngineNameStyle.Render(db.Engine+" "+displayVersion),
			authLabel,
		)

		tiles = append(tiles, TileStyle.Render(content))
	}

	if len(tiles) > 0 {
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, tiles...) + "\n\n")
	}

	for _, db := range result.Databases {
		if db.Type != dbType {
			continue
		}

		b.WriteString(EngineStyle.Render(db.Engine+" Databases") + "\n")
		// Filter default databases for display (but keep them for backup)
		displayNames := filterDefaultDatabases(db.Engine, db.Names)
		for _, name := range displayNames {
			b.WriteString(ItemStyle.Render("  • "+name) + "\n")
		}
		b.WriteString("\n")
	}
}

func renderDivider(b *strings.Builder) {
	b.WriteString(DividerStyle.Render(strings.Repeat("─", 50)) + "\n\n")
}

// filterDefaultDatabases filters out default system databases from display
// but keeps them in the original list for backup purposes
func filterDefaultDatabases(engine string, names []string) []string {
	if engine != "MongoDB" {
		return names
	}
	
	var filtered []string
	for _, name := range names {
		if !isDefaultDatabase(engine, name) {
			filtered = append(filtered, name)
		}
	}
	
	return filtered
}

// isDefaultDatabase checks if a database is a default system database
func isDefaultDatabase(engine, name string) bool {
	if engine != "MongoDB" {
		return false
	}
	
	// Default MongoDB databases created during installation
	defaultDBs := map[string]bool{
		"admin":  true,
		"config": true,
		"local":  true,
	}
	
	return defaultDBs[name]
}
