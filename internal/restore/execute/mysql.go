package execute

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"mirrorvault/internal/backup/credentials"
	"mirrorvault/internal/restore/log"
	restoreplan "mirrorvault/internal/restore/plan"
	"mirrorvault/internal/restore/validate"
)

func restoreMySQL(
	restorePlan *restoreplan.RestorePlan,
	dumpPath string,
	dumpInfo *validate.DumpInfo,
	authCtx *credentials.AuthContext,
	logger *log.Logger,
	onProgress func(string, float64, string, error),
) error {
	logger.Info("Starting MySQL restore")

	password := ""
	if restorePlan.RequiresAuth {
		pwd, ok := authCtx.Get("MySQL")
		if !ok {
			return fmt.Errorf("missing MySQL credentials")
		}
		password = pwd
	}

	// Step 1: Analyze dump to find tables that will be restored
	onProgress("Analyzing dump", 0.4, "Extracting table information from dump...", nil)
	logger.Info("Analyzing dump to identify tables")
	dumpTables, err := validate.ExtractTablesFromDump(dumpPath, dumpInfo.Compressed, dumpInfo.Compression)
	if err != nil {
		logger.Warning(fmt.Sprintf("Failed to analyze dump tables, will use full database restore: %v", err))
		// Fall back to full database restore if analysis fails
		dumpTables = nil
	} else {
		logger.Info(fmt.Sprintf("Found %d tables in dump: %v", len(dumpTables), dumpTables))
	}

	// Step 2: Ensure database exists (don't drop it)
	onProgress("Preparing database", 0.5, "Ensuring database exists...", nil)
	logger.Info("Ensuring database exists")
	var createCmd *exec.Cmd
	if restorePlan.RequiresAuth {
		createCmd = exec.Command("sudo", "mysql", "-u", "root", "-p"+password, "-e", fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s;", restorePlan.Database))
	} else {
		createCmd = exec.Command("sudo", "mysql", "-u", "root", "-e", fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s;", restorePlan.Database))
	}
	createCmd.Stderr = os.Stderr
	if err := createCmd.Run(); err != nil {
		return fmt.Errorf("failed to ensure database exists: %w", err)
	}

	// Step 3: Drop ALL existing tables to completely replace database with dump state
	// This ensures the current database exactly matches the dump - no preservation
	onProgress("Preparing database", 0.55, "Dropping all existing tables...", nil)
	logger.Info("Dropping all existing tables to completely replace with dump state")
	
	// Get list of all current tables in database
	var listTablesCmd *exec.Cmd
	if restorePlan.RequiresAuth {
		listTablesCmd = exec.Command("sudo", "mysql", "-u", "root", "-p"+password, "-N", "-e", fmt.Sprintf("USE %s; SHOW TABLES;", restorePlan.Database))
	} else {
		listTablesCmd = exec.Command("sudo", "mysql", "-u", "root", "-N", "-e", fmt.Sprintf("USE %s; SHOW TABLES;", restorePlan.Database))
	}
	
	var tablesOut bytes.Buffer
	listTablesCmd.Stdout = &tablesOut
	listTablesCmd.Stderr = os.Stderr
	
	currentTables := []string{}
	if err := listTablesCmd.Run(); err == nil {
		// Parse current tables
		for _, line := range strings.Split(tablesOut.String(), "\n") {
			tableName := strings.TrimSpace(line)
			if tableName != "" {
				currentTables = append(currentTables, tableName)
			}
		}
		logger.Info(fmt.Sprintf("Found %d existing tables to drop: %v", len(currentTables), currentTables))
	}
	
	// Drop all existing tables
	if len(currentTables) > 0 {
		for _, tableName := range currentTables {
			var dropTableCmd *exec.Cmd
			if restorePlan.RequiresAuth {
				dropTableCmd = exec.Command("sudo", "mysql", "-u", "root", "-p"+password, "-e", fmt.Sprintf("USE %s; DROP TABLE IF EXISTS %s;", restorePlan.Database, tableName))
			} else {
				dropTableCmd = exec.Command("sudo", "mysql", "-u", "root", "-e", fmt.Sprintf("USE %s; DROP TABLE IF EXISTS %s;", restorePlan.Database, tableName))
			}
			dropTableCmd.Stderr = os.Stderr
			if err := dropTableCmd.Run(); err != nil {
				logger.Warning(fmt.Sprintf("Failed to drop table %s: %v", tableName, err))
			} else {
				logger.Info(fmt.Sprintf("Dropped table: %s", tableName))
			}
		}
		logger.Info(fmt.Sprintf("Dropped %d existing tables - database is now empty and ready for restore", len(currentTables)))
	} else {
		logger.Info("No existing tables found - database is already empty")
	}

	// Step 2: Restore from dump
	onProgress("Restoring data", 0.6, "Importing data from dump...", nil)
	logger.Info("Importing data from dump")

	// Check file size first
	fileInfo, err := os.Stat(dumpPath)
	if err != nil {
		return fmt.Errorf("failed to stat dump file: %w", err)
	}
	if fileInfo.Size() == 0 {
		return fmt.Errorf("dump file is empty")
	}

	// Open dump file
	file, err := os.Open(dumpPath)
	if err != nil {
		return fmt.Errorf("failed to open dump file: %w", err)
	}
	defer file.Close()

	var reader io.Reader = file

	// Handle compression - try gzip first, fall back to uncompressed if it fails
	if dumpInfo.Compressed && dumpInfo.Compression == "gz" {
		// Try to create gzip reader
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			// Not a valid gzip file, treat as uncompressed
			logger.Warning(fmt.Sprintf("File has .gz extension but is not gzipped (%v), treating as uncompressed", err))
			if _, err := file.Seek(0, 0); err != nil {
				return fmt.Errorf("failed to reset file position: %w", err)
			}
			reader = file
		} else {
			// Valid gzip file
			defer gzReader.Close()
			reader = gzReader
		}
	}

	// Create mysql command
	var restoreCmd *exec.Cmd
	if restorePlan.RequiresAuth {
		restoreCmd = exec.Command("sudo", "mysql", "-u", "root", "-p"+password, restorePlan.Database)
	} else {
		restoreCmd = exec.Command("sudo", "mysql", "-u", "root", restorePlan.Database)
	}

	restoreCmd.Stdin = reader
	var stderr bytes.Buffer
	restoreCmd.Stderr = &stderr

	if err := restoreCmd.Run(); err != nil {
		errMsg := stderr.String()
		if errMsg == "" {
			errMsg = err.Error()
		}
		return fmt.Errorf("failed to restore database: %v\n%s", err, errMsg)
	}

	logger.Info("MySQL restore completed successfully")
	return nil
}

func rollbackMySQL(
	restorePlan *restoreplan.RestorePlan,
	backupPath string,
	authCtx *credentials.AuthContext,
	logger *log.Logger,
) error {
	logger.Info("Starting MySQL rollback")

	password := ""
	if restorePlan.RequiresAuth {
		pwd, ok := authCtx.Get("MySQL")
		if !ok {
			return fmt.Errorf("missing MySQL credentials")
		}
		password = pwd
	}

	// Drop database
	var dropCmd *exec.Cmd
	if restorePlan.RequiresAuth {
		dropCmd = exec.Command("sudo", "mysql", "-u", "root", "-p"+password, "-e", fmt.Sprintf("DROP DATABASE IF EXISTS %s;", restorePlan.Database))
	} else {
		dropCmd = exec.Command("sudo", "mysql", "-u", "root", "-e", fmt.Sprintf("DROP DATABASE IF EXISTS %s;", restorePlan.Database))
	}
	dropCmd.Stderr = os.Stderr
	dropCmd.Run() // Ignore errors

	// Create database
	var createCmd *exec.Cmd
	if restorePlan.RequiresAuth {
		createCmd = exec.Command("sudo", "mysql", "-u", "root", "-p"+password, "-e", fmt.Sprintf("CREATE DATABASE %s;", restorePlan.Database))
	} else {
		createCmd = exec.Command("sudo", "mysql", "-u", "root", "-e", fmt.Sprintf("CREATE DATABASE %s;", restorePlan.Database))
	}
	createCmd.Stderr = os.Stderr
	if err := createCmd.Run(); err != nil {
		return fmt.Errorf("failed to create database: %w", err)
	}

	// Restore from backup
	file, err := os.Open(backupPath)
	if err != nil {
		return fmt.Errorf("failed to open backup file: %w", err)
	}
	defer file.Close()

	var restoreCmd *exec.Cmd
	if restorePlan.RequiresAuth {
		restoreCmd = exec.Command("sudo", "mysql", "-u", "root", "-p"+password, restorePlan.Database)
	} else {
		restoreCmd = exec.Command("sudo", "mysql", "-u", "root", restorePlan.Database)
	}

	restoreCmd.Stdin = file
	restoreCmd.Stderr = os.Stderr

	if err := restoreCmd.Run(); err != nil {
		return fmt.Errorf("failed to restore from backup: %w", err)
	}

	logger.Info("MySQL rollback completed successfully")
	return nil
}
