package execute

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"mirrorvault/internal/backup/credentials"
	"mirrorvault/internal/config"
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

	user := config.MySQLUser()
	host := config.MySQLHost()
	port := config.MySQLPort()
	useSudo := host == "" || host == "localhost" || host == "127.0.0.1"

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
	createArgs := []string{"mysql", "-u", user, "-e", fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s;", restorePlan.Database)}
	if host != "" {
		createArgs = append(createArgs, "-h", host)
	}
	if port != "" {
		createArgs = append(createArgs, "-P", port)
	}
	if restorePlan.RequiresAuth {
		createArgs = append(createArgs, "-p"+password)
	}
	if useSudo {
		createCmd = exec.Command("sudo", createArgs...)
	} else {
		createCmd = exec.Command(createArgs[0], createArgs[1:]...)
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
	listArgs := []string{"mysql", "-u", user, "-N", "-e", fmt.Sprintf("USE %s; SHOW TABLES;", restorePlan.Database)}
	if host != "" {
		listArgs = append(listArgs, "-h", host)
	}
	if port != "" {
		listArgs = append(listArgs, "-P", port)
	}
	if restorePlan.RequiresAuth {
		listArgs = append(listArgs, "-p"+password)
	}
	if useSudo {
		listTablesCmd = exec.Command("sudo", listArgs...)
	} else {
		listTablesCmd = exec.Command(listArgs[0], listArgs[1:]...)
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
			dropArgs := []string{"mysql", "-u", user, "-e", fmt.Sprintf("USE %s; DROP TABLE IF EXISTS %s;", restorePlan.Database, tableName)}
			if host != "" {
				dropArgs = append(dropArgs, "-h", host)
			}
			if port != "" {
				dropArgs = append(dropArgs, "-P", port)
			}
			if restorePlan.RequiresAuth {
				dropArgs = append(dropArgs, "-p"+password)
			}
			if useSudo {
				dropTableCmd = exec.Command("sudo", dropArgs...)
			} else {
				dropTableCmd = exec.Command(dropArgs[0], dropArgs[1:]...)
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

	reader, closeReader, usedFallback, err := validate.OpenDecompressedReaderBestEffort(dumpPath, dumpInfo)
	if err != nil {
		return fmt.Errorf("failed to open dump file: %w", err)
	}
	if usedFallback {
		logger.Warning("File has .gz extension but is not gzipped; treating as uncompressed")
	}
	defer closeReader()

	// Create mysql command
	var restoreCmd *exec.Cmd
	restoreArgs := []string{"mysql", "-u", user, restorePlan.Database}
	if host != "" {
		restoreArgs = append(restoreArgs, "-h", host)
	}
	if port != "" {
		restoreArgs = append(restoreArgs, "-P", port)
	}
	if restorePlan.RequiresAuth {
		restoreArgs = append(restoreArgs, "-p"+password)
	}
	if useSudo {
		restoreCmd = exec.Command("sudo", restoreArgs...)
	} else {
		restoreCmd = exec.Command(restoreArgs[0], restoreArgs[1:]...)
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

	user := config.MySQLUser()
	host := config.MySQLHost()
	port := config.MySQLPort()
	useSudo := host == "" || host == "localhost" || host == "127.0.0.1"

	// Drop database
	var dropCmd *exec.Cmd
	dropArgs := []string{"mysql", "-u", user, "-e", fmt.Sprintf("DROP DATABASE IF EXISTS %s;", restorePlan.Database)}
	if host != "" {
		dropArgs = append(dropArgs, "-h", host)
	}
	if port != "" {
		dropArgs = append(dropArgs, "-P", port)
	}
	if restorePlan.RequiresAuth {
		dropArgs = append(dropArgs, "-p"+password)
	}
	if useSudo {
		dropCmd = exec.Command("sudo", dropArgs...)
	} else {
		dropCmd = exec.Command(dropArgs[0], dropArgs[1:]...)
	}
	dropCmd.Stderr = os.Stderr
	dropCmd.Run() // Ignore errors

	// Create database
	var createCmd *exec.Cmd
	createArgs := []string{"mysql", "-u", user, "-e", fmt.Sprintf("CREATE DATABASE %s;", restorePlan.Database)}
	if host != "" {
		createArgs = append(createArgs, "-h", host)
	}
	if port != "" {
		createArgs = append(createArgs, "-P", port)
	}
	if restorePlan.RequiresAuth {
		createArgs = append(createArgs, "-p"+password)
	}
	if useSudo {
		createCmd = exec.Command("sudo", createArgs...)
	} else {
		createCmd = exec.Command(createArgs[0], createArgs[1:]...)
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
	restoreArgs := []string{"mysql", "-u", user, restorePlan.Database}
	if host != "" {
		restoreArgs = append(restoreArgs, "-h", host)
	}
	if port != "" {
		restoreArgs = append(restoreArgs, "-P", port)
	}
	if restorePlan.RequiresAuth {
		restoreArgs = append(restoreArgs, "-p"+password)
	}
	if useSudo {
		restoreCmd = exec.Command("sudo", restoreArgs...)
	} else {
		restoreCmd = exec.Command(restoreArgs[0], restoreArgs[1:]...)
	}

	restoreCmd.Stdin = file
	restoreCmd.Stderr = os.Stderr

	if err := restoreCmd.Run(); err != nil {
		return fmt.Errorf("failed to restore from backup: %w", err)
	}

	logger.Info("MySQL rollback completed successfully")
	return nil
}
