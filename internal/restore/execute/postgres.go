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

func restorePostgreSQL(
	restorePlan *restoreplan.RestorePlan,
	dumpPath string,
	dumpInfo *validate.DumpInfo,
	authCtx *credentials.AuthContext,
	logger *log.Logger,
	onProgress func(string, float64, string, error),
) error {
	logger.Info("Starting PostgreSQL restore")

	password := ""
	if restorePlan.RequiresAuth {
		pwd, ok := authCtx.Get("PostgreSQL")
		if !ok {
			return fmt.Errorf("missing PostgreSQL credentials")
		}
		password = pwd
	}

	// Step 1: Analyze dump to find tables that will be restored
	onProgress("Analyzing dump", 0.4, "Extracting table information from dump...", nil)
	logger.Info("Analyzing dump to identify tables")
	dumpTables, err := validate.ExtractTablesFromDump(dumpPath, dumpInfo.Compressed, dumpInfo.Compression)
	if err != nil {
		logger.Warning(fmt.Sprintf("Failed to analyze dump tables, will use full database restore: %v", err))
		dumpTables = nil
	} else {
		logger.Info(fmt.Sprintf("Found %d tables in dump: %v", len(dumpTables), dumpTables))
	}

	// Step 2: Ensure database exists (don't drop it)
	onProgress("Preparing database", 0.5, "Ensuring database exists...", nil)
	logger.Info("Ensuring database exists")
	
	// Terminate existing connections first
	var termCmd *exec.Cmd
	if restorePlan.RequiresAuth {
		termCmd = exec.Command("sudo", "-u", "postgres", "psql", "-c", fmt.Sprintf("SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s' AND pid <> pg_backend_pid();", restorePlan.Database))
		termCmd.Env = append(termCmd.Env, fmt.Sprintf("PGPASSWORD=%s", password))
	} else {
		termCmd = exec.Command("sudo", "-u", "postgres", "psql", "-c", fmt.Sprintf("SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s' AND pid <> pg_backend_pid();", restorePlan.Database))
	}
	termCmd.Stderr = os.Stderr
	termCmd.Run() // Ignore errors

	var createCmd *exec.Cmd
	if restorePlan.RequiresAuth {
		createCmd = exec.Command("sudo", "-u", "postgres", "psql", "-c", fmt.Sprintf("CREATE DATABASE %s;", restorePlan.Database))
		createCmd.Env = append(createCmd.Env, fmt.Sprintf("PGPASSWORD=%s", password))
	} else {
		createCmd = exec.Command("sudo", "-u", "postgres", "psql", "-c", fmt.Sprintf("CREATE DATABASE %s;", restorePlan.Database))
	}
	createCmd.Stderr = os.Stderr
	createCmd.Run() // Ignore errors if database already exists

	// Step 3: Drop ALL existing tables to completely replace database with dump state
	// This ensures the current database exactly matches the dump - no preservation
	onProgress("Preparing database", 0.55, "Dropping all existing tables...", nil)
	logger.Info("Dropping all existing tables to completely replace with dump state")
	
	// Get list of all current tables in database
	var listTablesCmd *exec.Cmd
	if restorePlan.RequiresAuth {
		listTablesCmd = exec.Command("sudo", "-u", "postgres", "psql", "-d", restorePlan.Database, "-t", "-A", "-c", "SELECT tablename FROM pg_tables WHERE schemaname = 'public';")
		listTablesCmd.Env = append(listTablesCmd.Env, fmt.Sprintf("PGPASSWORD=%s", password))
	} else {
		listTablesCmd = exec.Command("sudo", "-u", "postgres", "psql", "-d", restorePlan.Database, "-t", "-A", "-c", "SELECT tablename FROM pg_tables WHERE schemaname = 'public';")
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
				dropTableCmd = exec.Command("sudo", "-u", "postgres", "psql", "-d", restorePlan.Database, "-c", fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE;", tableName))
				dropTableCmd.Env = append(dropTableCmd.Env, fmt.Sprintf("PGPASSWORD=%s", password))
			} else {
				dropTableCmd = exec.Command("sudo", "-u", "postgres", "psql", "-d", restorePlan.Database, "-c", fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE;", tableName))
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

	// Check file exists and get info
	fileInfo, err := os.Stat(dumpPath)
	if err != nil {
		return fmt.Errorf("failed to access dump file '%s': %w (check file path and permissions)", dumpPath, err)
	}
	
	// Warn if file is empty, but don't fail - let the restore attempt proceed
	// The restore will fail naturally if the file is truly empty
	if fileInfo.Size() == 0 {
		logger.Warning(fmt.Sprintf("Dump file '%s' appears to be empty (size: 0 bytes), but proceeding with restore attempt", dumpPath))
	} else {
		logger.Info(fmt.Sprintf("Dump file size: %d bytes", fileInfo.Size()))
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

	// Use psql to restore
	var restoreCmd *exec.Cmd
	if restorePlan.RequiresAuth {
		restoreCmd = exec.Command("sudo", "-u", "postgres", "psql", "-d", restorePlan.Database)
		restoreCmd.Env = append(restoreCmd.Env, fmt.Sprintf("PGPASSWORD=%s", password))
	} else {
		restoreCmd = exec.Command("sudo", "-u", "postgres", "psql", "-d", restorePlan.Database)
	}

	restoreCmd.Stdin = reader
	var stderr bytes.Buffer
	var stdout bytes.Buffer
	restoreCmd.Stderr = &stderr
	restoreCmd.Stdout = &stdout

	if err := restoreCmd.Run(); err != nil {
		errMsg := stderr.String()
		if errMsg == "" {
			errMsg = err.Error()
		}
		return fmt.Errorf("failed to restore database: %v\n%s", err, errMsg)
	}

	// Check if restore actually imported any data by checking stderr/stdout for warnings
	output := stderr.String() + stdout.String()
	if strings.Contains(output, "No commands were executed") || strings.Contains(output, "no data") {
		logger.Warning("Restore completed but may not have imported any data - check dump file")
	}

	logger.Info("PostgreSQL restore completed successfully")
	return nil
}

func rollbackPostgreSQL(
	restorePlan *restoreplan.RestorePlan,
	backupPath string,
	authCtx *credentials.AuthContext,
	logger *log.Logger,
) error {
	logger.Info("Starting PostgreSQL rollback")

	password := ""
	if restorePlan.RequiresAuth {
		pwd, ok := authCtx.Get("PostgreSQL")
		if !ok {
			return fmt.Errorf("missing PostgreSQL credentials")
		}
		password = pwd
	}

	// Terminate connections
	var termCmd *exec.Cmd
	if restorePlan.RequiresAuth {
		termCmd = exec.Command("sudo", "-u", "postgres", "psql", "-c", fmt.Sprintf("SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s' AND pid <> pg_backend_pid();", restorePlan.Database))
		termCmd.Env = append(termCmd.Env, fmt.Sprintf("PGPASSWORD=%s", password))
	} else {
		termCmd = exec.Command("sudo", "-u", "postgres", "psql", "-c", fmt.Sprintf("SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s' AND pid <> pg_backend_pid();", restorePlan.Database))
	}
	termCmd.Stderr = os.Stderr
	termCmd.Run()

	// Drop database
	var dropCmd *exec.Cmd
	if restorePlan.RequiresAuth {
		dropCmd = exec.Command("sudo", "-u", "postgres", "psql", "-c", fmt.Sprintf("DROP DATABASE IF EXISTS %s;", restorePlan.Database))
		dropCmd.Env = append(dropCmd.Env, fmt.Sprintf("PGPASSWORD=%s", password))
	} else {
		dropCmd = exec.Command("sudo", "-u", "postgres", "psql", "-c", fmt.Sprintf("DROP DATABASE IF EXISTS %s;", restorePlan.Database))
	}
	dropCmd.Stderr = os.Stderr
	dropCmd.Run()

	// Create database
	var createCmd *exec.Cmd
	if restorePlan.RequiresAuth {
		createCmd = exec.Command("sudo", "-u", "postgres", "psql", "-c", fmt.Sprintf("CREATE DATABASE %s;", restorePlan.Database))
		createCmd.Env = append(createCmd.Env, fmt.Sprintf("PGPASSWORD=%s", password))
	} else {
		createCmd = exec.Command("sudo", "-u", "postgres", "psql", "-c", fmt.Sprintf("CREATE DATABASE %s;", restorePlan.Database))
	}
	createCmd.Stderr = os.Stderr
	if err := createCmd.Run(); err != nil {
		return fmt.Errorf("failed to create database: %w", err)
	}

	// Verify backup file exists before attempting restore
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("backup file does not exist at: %s", backupPath)
	}

	logger.Info(fmt.Sprintf("Restoring from backup file: %s", backupPath))

	// Restore from backup
	file, err := os.Open(backupPath)
	if err != nil {
		return fmt.Errorf("failed to open backup file %s: %w", backupPath, err)
	}
	defer file.Close()

	var restoreCmd *exec.Cmd
	if restorePlan.RequiresAuth {
		restoreCmd = exec.Command("sudo", "-u", "postgres", "psql", "-d", restorePlan.Database)
		restoreCmd.Env = append(restoreCmd.Env, fmt.Sprintf("PGPASSWORD=%s", password))
	} else {
		restoreCmd = exec.Command("sudo", "-u", "postgres", "psql", "-d", restorePlan.Database)
	}

	restoreCmd.Stdin = file
	var stderr bytes.Buffer
	restoreCmd.Stderr = &stderr

	if err := restoreCmd.Run(); err != nil {
		errMsg := stderr.String()
		if errMsg == "" {
			errMsg = err.Error()
		}
		return fmt.Errorf("failed to restore from backup: %v\n%s", err, errMsg)
	}

	logger.Info("Successfully restored database from backup")

	logger.Info("PostgreSQL rollback completed successfully")
	return nil
}
