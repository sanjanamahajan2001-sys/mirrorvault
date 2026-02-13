package schedule

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	BackupRetentionDays = 14
	BackupBaseDir       = "/var/backups/mirrorvault"
	DailyBackupDir      = "/var/backups/mirrorvault/daily-backups"
)

// CreateCleanupService creates the cleanup service and timer
func CreateCleanupService() error {
	// Find mirrorvault binary path
	mirrorvaultPath, err := findMirrorVaultPath()
	if err != nil {
		return fmt.Errorf("failed to find mirrorvault binary: %w", err)
	}

	// Create cleanup service unit
	serviceContent := fmt.Sprintf(`[Unit]
Description=MirrorVault backup cleanup - removes backups older than 14 days
After=network.target

[Service]
Type=oneshot
ExecStart=%s cleanup
User=root
`, mirrorvaultPath)

	// Create cleanup timer unit
	timerContent := fmt.Sprintf(`[Unit]
Description=MirrorVault daily cleanup timer
Requires=%s

[Timer]
OnCalendar=daily
OnCalendar=*-*-* 01:00:00
Persistent=true

[Install]
WantedBy=timers.target
`, CleanupServiceName)

	servicePath := filepath.Join(SystemdUnitDir, CleanupServiceName)
	timerPath := filepath.Join(SystemdUnitDir, CleanupTimerName)

	// Write service unit
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("failed to write cleanup service: %w", err)
	}

	// Write timer unit
	if err := os.WriteFile(timerPath, []byte(timerContent), 0644); err != nil {
		return fmt.Errorf("failed to write cleanup timer: %w", err)
	}

	// Reload systemd
	cmd := exec.Command("sudo", "systemctl", "daemon-reload")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	// Enable and start timer
	cmd = exec.Command("sudo", "systemctl", "enable", CleanupTimerName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to enable cleanup timer: %w", err)
	}

	cmd = exec.Command("sudo", "systemctl", "start", CleanupTimerName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start cleanup timer: %w", err)
	}

	return nil
}

// CreateCleanupCron creates a cron entry for cleanup on non-systemd systems
func CreateCleanupCron() error {
	mirrorvaultPath, err := findMirrorVaultPath()
	if err != nil {
		return fmt.Errorf("failed to find mirrorvault binary: %w", err)
	}

	cronPath := filepath.Join(cronDir, "mirrorvault-cleanup")
	content := fmt.Sprintf(`SHELL=/bin/sh
PATH=/usr/local/sbin:/usr/local/bin:/sbin:/bin:/usr/sbin:/usr/bin
00 01 * * * root "%s" cleanup
`, mirrorvaultPath)

	if err := os.WriteFile(cronPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write cleanup cron file: %w", err)
	}

	return nil
}

// RunCleanup removes backups older than 14 days
func RunCleanup() error {
	cutoffDate := time.Now().AddDate(0, 0, -BackupRetentionDays)

	// Clean up both manual backups and daily backups
	dirs := []string{BackupBaseDir, DailyBackupDir}

	for _, baseDir := range dirs {
		if err := cleanupDirectory(baseDir, cutoffDate); err != nil {
			// Log error but continue with other directories
			fmt.Printf("Warning: error cleaning %s: %v\n", baseDir, err)
		}
	}

	return nil
}

// cleanupDirectory removes backups older than cutoffDate from a directory
func cleanupDirectory(baseDir string, cutoffDate time.Time) error {
	// Walk through all backup directories
	return filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors, continue cleanup
		}

		// Skip the base directories themselves
		if path == BackupBaseDir || path == DailyBackupDir {
			return nil
		}

		// Skip the daily-backups directory itself (don't delete the directory, just its contents)
		if path == DailyBackupDir {
			return nil
		}

		// Check if file/directory name contains a date
		// Format: name_YYYY-MM-DD.ext or name_YYYY-MM-DD/
		baseName := filepath.Base(path)

		// Extract date from filename (format: *_YYYY-MM-DD.*)
		dateStr := extractDateFromName(baseName)
		if dateStr == "" {
			// If no date found, check if it's a directory that might contain dated backups
			if info.IsDir() {
				return nil // Will be handled when we walk into it
			}
			return nil // Skip files without dates
		}

		// Parse the date
		backupDate, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return nil // Skip if date parsing fails
		}

		// Check if backup is older than retention period
		if backupDate.Before(cutoffDate) {
			fmt.Printf("Removing old backup: %s (date: %s)\n", path, dateStr)
			if err := os.RemoveAll(path); err != nil {
				fmt.Printf("Warning: failed to remove %s: %v\n", path, err)
			}
		}

		return nil
	})
}

// extractDateFromName extracts date in YYYY-MM-DD format from filename
// Examples: "app_db_2026-01-08.sql" -> "2026-01-08"
//           "demo_mongo_2026-01-08" -> "2026-01-08"
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
			if len(parts[i]) > 10 && parts[i][:10] != "" {
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
