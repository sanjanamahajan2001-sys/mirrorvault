package history

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const LogDir = "/var/log/mirrorvault"

type RestoreHistory struct {
	Timestamp        time.Time
	Engine           string
	Database         string
	DumpPath         string
	DumpFormat       string
	Compressed       bool
	MultiDB          bool
	PreRestoreBackup string
	Success          bool
	RolledBack       bool
	Error            string
	LogFile          string
}

// ParseLogFile extracts restore information from a log file
func ParseLogFile(logPath string) (*RestoreHistory, error) {
	file, err := os.Open(logPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	history := &RestoreHistory{
		LogFile: logPath,
	}

	scanner := bufio.NewScanner(file)
	
	// Extract timestamp from filename: restore_Engine_Database_YYYY-MM-DD_HH-MM-SS.log
	filename := filepath.Base(logPath)
	timeMatch := regexp.MustCompile(`restore_.*_(\d{4}-\d{2}-\d{2}_\d{2}-\d{2}-\d{2})\.log`).FindStringSubmatch(filename)
	if len(timeMatch) > 1 {
		if t, err := time.Parse("2006-01-02_15-04-05", timeMatch[1]); err == nil {
			history.Timestamp = t
		}
	}

	// Extract engine and database from filename or log content
	engineDBMatch := regexp.MustCompile(`restore_([^_]+)_(.+?)_\d{4}-\d{2}-\d{2}_\d{2}-\d{2}-\d{2}\.log`).FindStringSubmatch(filename)
	if len(engineDBMatch) >= 3 {
		history.Engine = engineDBMatch[1]
		// Database name might have underscores, so we need to reconstruct it
		// The pattern is: restore_Engine_DatabaseName_YYYY-MM-DD_HH-MM-SS.log
		// We need to extract everything between Engine and the timestamp
		parts := strings.Split(filename, "_")
		if len(parts) >= 4 {
			// Find where the date starts (format: YYYY-MM-DD)
			for i := 2; i < len(parts); i++ {
				if matched, _ := regexp.MatchString(`^\d{4}-\d{2}-\d{2}$`, parts[i]); matched {
					// Reconstruct database name from parts[2] to parts[i-1]
					history.Database = strings.Join(parts[2:i], "_")
					break
				}
			}
		}
	}

	// Parse log content
	for scanner.Scan() {
		line := scanner.Text()
		
		// Extract engine and database from log content if not found in filename
		if strings.Contains(line, "Starting restore operation for") {
			match := regexp.MustCompile(`Starting restore operation for ([^/]+)/(.+)`).FindStringSubmatch(line)
			if len(match) >= 3 {
				history.Engine = match[1]
				history.Database = strings.TrimSpace(match[2])
			}
		}
		
		if strings.Contains(line, "Dump path:") {
			match := regexp.MustCompile(`Dump path: (.+)`).FindStringSubmatch(line)
			if len(match) >= 2 {
				history.DumpPath = strings.TrimSpace(match[1])
			}
		}
		
		if strings.Contains(line, "Dump format:") {
			match := regexp.MustCompile(`Dump format: ([^,]+), Compressed: (true|false), Multi-DB: (true|false)`).FindStringSubmatch(line)
			if len(match) >= 4 {
				history.DumpFormat = strings.TrimSpace(match[1])
				history.Compressed = match[2] == "true"
				history.MultiDB = match[3] == "true"
			}
		}
		
		if strings.Contains(line, "Pre-restore backup created:") || strings.Contains(line, "Pre-restore backup created at:") {
			match := regexp.MustCompile(`Pre-restore backup (created|created at): (.+)`).FindStringSubmatch(line)
			if len(match) >= 3 {
				history.PreRestoreBackup = strings.TrimSpace(match[2])
			}
		}
		
		if strings.Contains(line, "Restore operation completed successfully") {
			history.Success = true
		}
		
		if strings.Contains(line, "Restore failed:") {
			history.Success = false
			match := regexp.MustCompile(`Restore failed: (.+)`).FindStringSubmatch(line)
			if len(match) >= 2 {
				history.Error = strings.TrimSpace(match[1])
			}
		}
		
		if strings.Contains(line, "Initiating automatic rollback") || strings.Contains(line, "Rollback completed successfully") {
			history.RolledBack = true
		}
		
		// Extract error messages
		if strings.Contains(line, "[ERROR]") {
			parts := strings.SplitN(line, "[ERROR]", 2)
			if len(parts) == 2 {
				errorMsg := strings.TrimSpace(parts[1])
				if history.Error == "" {
					history.Error = errorMsg
				} else {
					history.Error += "; " + errorMsg
				}
			}
		}
	}

	// If timestamp not found in filename, try to extract from first log line
	if history.Timestamp.IsZero() {
		file.Seek(0, 0)
		scanner = bufio.NewScanner(file)
		if scanner.Scan() {
			line := scanner.Text()
			// Try to parse timestamp from log line: [YYYY-MM-DD HH:MM:SS]
			timeMatch := regexp.MustCompile(`\[(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})\]`).FindStringSubmatch(line)
			if len(timeMatch) > 1 {
				if t, err := time.Parse("2006-01-02 15:04:05", timeMatch[1]); err == nil {
					history.Timestamp = t
				}
			}
		}
	}

	return history, scanner.Err()
}

// GetAllRestoreHistory reads all restore log files and returns a list of restore histories
func GetAllRestoreHistory() ([]*RestoreHistory, error) {
	// Ensure log directory exists
	if _, err := os.Stat(LogDir); os.IsNotExist(err) {
		return []*RestoreHistory{}, nil
	}

	var histories []*RestoreHistory

	err := filepath.Walk(LogDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Only process restore log files
		if !info.IsDir() && strings.HasPrefix(info.Name(), "restore_") && strings.HasSuffix(info.Name(), ".log") {
			history, err := ParseLogFile(path)
			if err != nil {
				// Skip files that can't be parsed
				return nil
			}
			histories = append(histories, history)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to read log directory: %w", err)
	}

	// Sort by timestamp (newest first)
	for i := 0; i < len(histories)-1; i++ {
		for j := i + 1; j < len(histories); j++ {
			if histories[i].Timestamp.Before(histories[j].Timestamp) {
				histories[i], histories[j] = histories[j], histories[i]
			}
		}
	}

	return histories, nil
}

// LoadRestoreHistory is a convenience function that loads all restore history
func LoadRestoreHistory() ([]*RestoreHistory, error) {
	return GetAllRestoreHistory()
}
