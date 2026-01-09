package execute

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"compress/gzip"

	"mirrorvault/internal/restore/log"
	restoreplan "mirrorvault/internal/restore/plan"
	"mirrorvault/internal/restore/validate"
)

func restoreSQLite(
	restorePlan *restoreplan.RestorePlan,
	dumpPath string,
	dumpInfo *validate.DumpInfo,
	logger *log.Logger,
	onProgress func(string, float64, string, error),
) error {
	logger.Info("Starting SQLite restore")

	// SQLite database is a file, not a server
	// We need to find the actual database file path
	dbPath := restorePlan.Database

	// Step 1: Remove existing database to completely replace with dump state
	// The pre-restore backup is already created by the executor, so we can safely remove
	onProgress("Preparing database", 0.5, "Removing existing database...", nil)
	logger.Info("Removing existing database to completely replace with dump state")
	if _, err := os.Stat(dbPath); err == nil {
		if err := os.Remove(dbPath); err != nil {
			logger.Warning(fmt.Sprintf("Failed to remove existing database file: %v", err))
		} else {
			logger.Info("Removed existing database file - will be completely replaced with dump")
		}
	}

	// Step 2: Restore from dump
	onProgress("Restoring data", 0.6, "Importing data from dump...", nil)
	logger.Info("Importing data from dump")

	// Open dump file
	file, err := os.Open(dumpPath)
	if err != nil {
		return fmt.Errorf("failed to open dump file: %w", err)
	}
	defer file.Close()

	var reader io.Reader = file

	// Handle compression
	if dumpInfo.Compressed && dumpInfo.Compression == "gz" {
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			return fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}

	// Create new database file
	dbFile, err := os.Create(dbPath)
	if err != nil {
		return fmt.Errorf("failed to create database file: %w", err)
	}
	defer dbFile.Close()

	// Read dump and write to database file
	_, err = io.Copy(dbFile, reader)
	if err != nil {
		return fmt.Errorf("failed to write database file: %w", err)
	}

	// For SQL dumps, we need to import using sqlite3
	// Check if dump is SQL format
	if dumpInfo.Format == "sql" {
		// Close the file we just created
		dbFile.Close()

		// Remove the file we created
		os.Remove(dbPath)

		// Reopen dump file
		file2, err := os.Open(dumpPath)
		if err != nil {
			return fmt.Errorf("failed to reopen dump file: %w", err)
		}
		defer file2.Close()

		var reader2 io.Reader = file2
		if dumpInfo.Compressed && dumpInfo.Compression == "gz" {
			gzReader, err := gzip.NewReader(file2)
			if err != nil {
				return fmt.Errorf("failed to create gzip reader: %w", err)
			}
			defer gzReader.Close()
			reader2 = gzReader
		}

		// Use sqlite3 to import
		cmd := exec.Command("sqlite3", dbPath)
		cmd.Stdin = reader2
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to import SQL dump: %w", err)
		}
	}

	logger.Info("SQLite restore completed successfully")
	return nil
}

func rollbackSQLite(
	restorePlan *restoreplan.RestorePlan,
	backupPath string,
	logger *log.Logger,
) error {
	logger.Info("Starting SQLite rollback")

	dbPath := restorePlan.Database

	// Remove current database
	os.Remove(dbPath)

	// Restore from backup
	if err := copyFile(backupPath, dbPath); err != nil {
		return fmt.Errorf("failed to restore from backup: %w", err)
	}

	logger.Info("SQLite rollback completed successfully")
	return nil
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}
