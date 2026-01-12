package log

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const LogDir = "/var/log/mirrorvault"

type Logger struct {
	file   *os.File
	prefix string
}

func NewLogger(operation string) (*Logger, error) {
	// Ensure log directory exists
	if err := os.MkdirAll(LogDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Sanitize operation string for filename (replace invalid characters)
	// For SQLite, database name is a full path like /home/user/db.db
	// We need to replace slashes and other invalid filename characters
	sanitizedOp := sanitizeForFilename(operation)

	// Create log file with timestamp
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	filename := fmt.Sprintf("restore_%s_%s.log", sanitizedOp, timestamp)
	logPath := filepath.Join(LogDir, filename)

	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to create log file: %w", err)
	}

	logger := &Logger{
		file:   file,
		prefix: fmt.Sprintf("[%s] ", time.Now().Format("2006-01-02 15:04:05")),
	}

	// Write header
	logger.Info("=== MirrorVault Restore Operation Started ===")
	logger.Info(fmt.Sprintf("Operation: %s", operation))
	logger.Info(fmt.Sprintf("Log File: %s", logPath))

	return logger, nil
}

func (l *Logger) Info(message string) {
	l.write("INFO", message)
}

func (l *Logger) Error(message string) {
	l.write("ERROR", message)
}

func (l *Logger) Warning(message string) {
	l.write("WARNING", message)
}

func (l *Logger) write(level, message string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logLine := fmt.Sprintf("[%s] [%s] %s\n", timestamp, level, message)
	l.file.WriteString(logLine)
	
	// Note: We do NOT print to stdout here because when running in Bubble Tea TUI mode,
	// direct stdout writes interfere with the terminal rendering and cause alignment issues.
	// Logs are still written to the log file for debugging and history.
	// If you need to display logs in the TUI, send them through Bubble Tea's message system instead.
}

func (l *Logger) Close() error {
	if l.file != nil {
		l.Info("=== MirrorVault Restore Operation Completed ===")
		return l.file.Close()
	}
	return nil
}

// sanitizeForFilename replaces invalid filename characters with safe alternatives
func sanitizeForFilename(name string) string {
	// Replace path separators with underscores
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	// Replace other invalid characters
	name = strings.ReplaceAll(name, ":", "_")
	name = strings.ReplaceAll(name, "*", "_")
	name = strings.ReplaceAll(name, "?", "_")
	name = strings.ReplaceAll(name, "\"", "_")
	name = strings.ReplaceAll(name, "<", "_")
	name = strings.ReplaceAll(name, ">", "_")
	name = strings.ReplaceAll(name, "|", "_")
	// Remove leading/trailing spaces and dots (Windows doesn't allow these)
	name = strings.Trim(name, " .")
	// Limit length to avoid filesystem issues
	if len(name) > 200 {
		name = name[:200]
	}
	return name
}
