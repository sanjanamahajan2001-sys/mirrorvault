package restore

// This file provides utilities for finding backups

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const DefaultBackupDir = "/var/backups/mirrorvault"
const DailyBackupDir = "/var/backups/mirrorvault/daily-backups"

// FindLatestBackup finds the most recent backup for a given engine and database
func FindLatestBackup(engine, database string) (string, error) {
	// Check both manual and daily backup directories
	dirs := []string{
		filepath.Join(DefaultBackupDir, strings.ToLower(engine)),
		filepath.Join(DailyBackupDir, strings.ToLower(engine)),
	}

	type backupFile struct {
		Path string
		Date time.Time
	}

	var backups []backupFile

	// Determine search prefixes based on engine and database name
	searchPrefixes := []string{}
	
	if engine == "Redis" && database == "dump.rdb" {
		// Redis backups are named redis_YYYY-MM-DD.rdb, not dump.rdb_YYYY-MM-DD.rdb
		searchPrefixes = append(searchPrefixes, "redis_")
	} else if engine == "SQLite" {
		// SQLite database name is the full file path, but backups use base filename
		// e.g., database: /home/sanjana/demo_sqlite.db
		// backup: demo_sqlite_2026-01-09.sql
		baseName := filepath.Base(database)
		// Remove extension to get base name
		baseNameWithoutExt := strings.TrimSuffix(baseName, filepath.Ext(baseName))
		searchPrefixes = append(searchPrefixes, baseNameWithoutExt+"_")
		// Also try with the full base filename (with extension removed)
		searchPrefixes = append(searchPrefixes, baseName+"_")
	} else {
		// Default: use database name as prefix
		searchPrefixes = append(searchPrefixes, database+"_")
	}

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // Skip if directory doesn't exist
		}

		for _, entry := range entries {
			if entry.IsDir() {
				// MongoDB backups are directories
				for _, prefix := range searchPrefixes {
					if strings.HasPrefix(entry.Name(), prefix) {
						dateStr := extractDateFromName(entry.Name())
						if dateStr != "" {
							if date, err := time.Parse("2006-01-02", dateStr); err == nil {
								backups = append(backups, backupFile{
									Path: filepath.Join(dir, entry.Name()),
									Date: date,
								})
								break // Found a match, no need to check other prefixes
							}
						}
					}
				}
			} else {
				// File backups (SQL, Redis, etc.)
				for _, prefix := range searchPrefixes {
					if strings.HasPrefix(entry.Name(), prefix) {
						dateStr := extractDateFromName(entry.Name())
						if dateStr != "" {
							if date, err := time.Parse("2006-01-02", dateStr); err == nil {
								backups = append(backups, backupFile{
									Path: filepath.Join(dir, entry.Name()),
									Date: date,
								})
								break // Found a match, no need to check other prefixes
							}
						}
					}
				}
			}
		}
	}

	if len(backups) == 0 {
		// Provide more helpful error message with actual search locations
		searchDirs := []string{
			filepath.Join(DefaultBackupDir, strings.ToLower(engine)),
			filepath.Join(DailyBackupDir, strings.ToLower(engine)),
		}
		// For SQLite, show just the base filename in error (not full path)
		dbDisplayName := database
		if engine == "SQLite" {
			dbDisplayName = filepath.Base(database)
		}
		return "", fmt.Errorf("no backups found for %s/%s. Searched in: %v", engine, dbDisplayName, searchDirs)
	}

	// Sort by date (most recent first)
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Date.After(backups[j].Date)
	})

	return backups[0].Path, nil
}

func extractDateFromName(name string) string {
	// Look for pattern: _YYYY-MM-DD
	parts := strings.Split(name, "_")
	for i := len(parts) - 1; i >= 0; i-- {
		if len(parts[i]) >= 10 {
			// Check if it matches YYYY-MM-DD format
			if len(parts[i]) == 10 && strings.Count(parts[i], "-") == 2 {
				// Validate it's a date
				if _, err := time.Parse("2006-01-02", parts[i]); err == nil {
					return parts[i]
				}
			}
			// Also check if date is followed by extension
			if len(parts[i]) > 10 {
				datePart := parts[i][:10]
				if strings.Count(datePart, "-") == 2 {
					if _, err := time.Parse("2006-01-02", datePart); err == nil {
						return datePart
					}
				}
			}
		}
	}
	return ""
}
