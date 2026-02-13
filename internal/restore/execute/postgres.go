package execute

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"mirrorvault/internal/backup/credentials"
	"mirrorvault/internal/config"
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

	user := config.PostgresUser()
	host := config.PostgresHost()
	port := config.PostgresPort()
	useSudo := host == "" && user == "postgres"

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
	termArgs := []string{"psql", "-c", fmt.Sprintf("SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s' AND pid <> pg_backend_pid();", restorePlan.Database)}
	if user != "" {
		termArgs = append([]string{termArgs[0], "-U", user}, termArgs[1:]...)
	}
	if host != "" {
		termArgs = append(termArgs, "-h", host)
	}
	if port != "" {
		termArgs = append(termArgs, "-p", port)
	}
	if useSudo {
		termCmd = exec.Command("sudo", append([]string{"-u", "postgres"}, termArgs...)...)
	} else {
		termCmd = exec.Command(termArgs[0], termArgs[1:]...)
	}
	if restorePlan.RequiresAuth {
		termCmd.Env = append(termCmd.Env, fmt.Sprintf("PGPASSWORD=%s", password))
	}
	termCmd.Stderr = os.Stderr
	termCmd.Run() // Ignore errors

	var createCmd *exec.Cmd
	createArgs := []string{"psql", "-c", fmt.Sprintf("CREATE DATABASE %s;", restorePlan.Database)}
	if user != "" {
		createArgs = append([]string{createArgs[0], "-U", user}, createArgs[1:]...)
	}
	if host != "" {
		createArgs = append(createArgs, "-h", host)
	}
	if port != "" {
		createArgs = append(createArgs, "-p", port)
	}
	if useSudo {
		createCmd = exec.Command("sudo", append([]string{"-u", "postgres"}, createArgs...)...)
	} else {
		createCmd = exec.Command(createArgs[0], createArgs[1:]...)
	}
	if restorePlan.RequiresAuth {
		createCmd.Env = append(createCmd.Env, fmt.Sprintf("PGPASSWORD=%s", password))
	}
	createCmd.Stderr = os.Stderr
	createCmd.Run() // Ignore errors if database already exists

	// Step 3: Drop ALL existing tables to completely replace database with dump state
	// This ensures the current database exactly matches the dump - no preservation
	onProgress("Preparing database", 0.55, "Dropping all existing tables...", nil)
	logger.Info("Dropping all existing tables to completely replace with dump state")

	// Get list of all current tables in database
	var listTablesCmd *exec.Cmd
	listArgs := []string{"psql", "-d", restorePlan.Database, "-t", "-A", "-c", "SELECT tablename FROM pg_tables WHERE schemaname = 'public';"}
	if user != "" {
		listArgs = append([]string{listArgs[0], "-U", user}, listArgs[1:]...)
	}
	if host != "" {
		listArgs = append(listArgs, "-h", host)
	}
	if port != "" {
		listArgs = append(listArgs, "-p", port)
	}
	if useSudo {
		listTablesCmd = exec.Command("sudo", append([]string{"-u", "postgres"}, listArgs...)...)
	} else {
		listTablesCmd = exec.Command(listArgs[0], listArgs[1:]...)
	}
	if restorePlan.RequiresAuth {
		listTablesCmd.Env = append(listTablesCmd.Env, fmt.Sprintf("PGPASSWORD=%s", password))
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
			dropArgs := []string{"psql", "-d", restorePlan.Database, "-c", fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE;", tableName)}
			if user != "" {
				dropArgs = append([]string{dropArgs[0], "-U", user}, dropArgs[1:]...)
			}
			if host != "" {
				dropArgs = append(dropArgs, "-h", host)
			}
			if port != "" {
				dropArgs = append(dropArgs, "-p", port)
			}
			if useSudo {
				dropTableCmd = exec.Command("sudo", append([]string{"-u", "postgres"}, dropArgs...)...)
			} else {
				dropTableCmd = exec.Command(dropArgs[0], dropArgs[1:]...)
			}
			if restorePlan.RequiresAuth {
				dropTableCmd.Env = append(dropTableCmd.Env, fmt.Sprintf("PGPASSWORD=%s", password))
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

	if dumpInfo.Format == "postgres_custom" || dumpInfo.Format == "postgres_dir" {
		if _, err := exec.LookPath("pg_restore"); err != nil {
			return fmt.Errorf("required command not found: pg_restore")
		}
		restorePath := dumpPath
		cleanup := func() {}
		if dumpInfo.Format == "postgres_custom" && dumpInfo.Compressed {
			var err error
			var usedFallback bool
			restorePath, cleanup, usedFallback, err = writeDecompressedTempFile(dumpPath, dumpInfo, ".dump")
			if err != nil {
				return err
			}
			if usedFallback {
				logger.Warning("File has .gz extension but is not gzipped; treating as uncompressed")
			}
		}
		defer cleanup()

		var restoreCmd *exec.Cmd
		restoreArgs := []string{"pg_restore", "--no-owner", "--no-acl", "-d", restorePlan.Database}
		if user != "" {
			restoreArgs = append([]string{restoreArgs[0], "-U", user}, restoreArgs[1:]...)
		}
		if host != "" {
			restoreArgs = append(restoreArgs, "-h", host)
		}
		if port != "" {
			restoreArgs = append(restoreArgs, "-p", port)
		}
		restoreArgs = append(restoreArgs, restorePath)
		if useSudo {
			restoreCmd = exec.Command("sudo", append([]string{"-u", "postgres"}, restoreArgs...)...)
		} else {
			restoreCmd = exec.Command(restoreArgs[0], restoreArgs[1:]...)
		}
		if restorePlan.RequiresAuth {
			restoreCmd.Env = append(restoreCmd.Env, fmt.Sprintf("PGPASSWORD=%s", password))
		}

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

		logger.Info("PostgreSQL restore completed successfully")
		return nil
	}

	reader, closeReader, usedFallback, err := validate.OpenDecompressedReaderBestEffort(dumpPath, dumpInfo)
	if err != nil {
		return fmt.Errorf("failed to open dump file: %w", err)
	}
	if usedFallback {
		logger.Warning("File has .gz extension but is not gzipped; treating as uncompressed")
	}
	defer closeReader()

	// Use psql to restore
	var restoreCmd *exec.Cmd
	restoreArgs := []string{"psql", "-d", restorePlan.Database}
	if user != "" {
		restoreArgs = append([]string{restoreArgs[0], "-U", user}, restoreArgs[1:]...)
	}
	if host != "" {
		restoreArgs = append(restoreArgs, "-h", host)
	}
	if port != "" {
		restoreArgs = append(restoreArgs, "-p", port)
	}
	if useSudo {
		restoreCmd = exec.Command("sudo", append([]string{"-u", "postgres"}, restoreArgs...)...)
	} else {
		restoreCmd = exec.Command(restoreArgs[0], restoreArgs[1:]...)
	}
	if restorePlan.RequiresAuth {
		restoreCmd.Env = append(restoreCmd.Env, fmt.Sprintf("PGPASSWORD=%s", password))
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

	user := config.PostgresUser()
	host := config.PostgresHost()
	port := config.PostgresPort()
	useSudo := host == "" && user == "postgres"

	// Terminate connections
	var termCmd *exec.Cmd
	termArgs := []string{"psql", "-c", fmt.Sprintf("SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s' AND pid <> pg_backend_pid();", restorePlan.Database)}
	if user != "" {
		termArgs = append([]string{termArgs[0], "-U", user}, termArgs[1:]...)
	}
	if host != "" {
		termArgs = append(termArgs, "-h", host)
	}
	if port != "" {
		termArgs = append(termArgs, "-p", port)
	}
	if useSudo {
		termCmd = exec.Command("sudo", append([]string{"-u", "postgres"}, termArgs...)...)
	} else {
		termCmd = exec.Command(termArgs[0], termArgs[1:]...)
	}
	if restorePlan.RequiresAuth {
		termCmd.Env = append(termCmd.Env, fmt.Sprintf("PGPASSWORD=%s", password))
	}
	termCmd.Stderr = os.Stderr
	termCmd.Run()

	// Drop database
	var dropCmd *exec.Cmd
	dropArgs := []string{"psql", "-c", fmt.Sprintf("DROP DATABASE IF EXISTS %s;", restorePlan.Database)}
	if user != "" {
		dropArgs = append([]string{dropArgs[0], "-U", user}, dropArgs[1:]...)
	}
	if host != "" {
		dropArgs = append(dropArgs, "-h", host)
	}
	if port != "" {
		dropArgs = append(dropArgs, "-p", port)
	}
	if useSudo {
		dropCmd = exec.Command("sudo", append([]string{"-u", "postgres"}, dropArgs...)...)
	} else {
		dropCmd = exec.Command(dropArgs[0], dropArgs[1:]...)
	}
	if restorePlan.RequiresAuth {
		dropCmd.Env = append(dropCmd.Env, fmt.Sprintf("PGPASSWORD=%s", password))
	}
	dropCmd.Stderr = os.Stderr
	dropCmd.Run()

	// Create database
	var createCmd *exec.Cmd
	createArgs := []string{"psql", "-c", fmt.Sprintf("CREATE DATABASE %s;", restorePlan.Database)}
	if user != "" {
		createArgs = append([]string{createArgs[0], "-U", user}, createArgs[1:]...)
	}
	if host != "" {
		createArgs = append(createArgs, "-h", host)
	}
	if port != "" {
		createArgs = append(createArgs, "-p", port)
	}
	if useSudo {
		createCmd = exec.Command("sudo", append([]string{"-u", "postgres"}, createArgs...)...)
	} else {
		createCmd = exec.Command(createArgs[0], createArgs[1:]...)
	}
	if restorePlan.RequiresAuth {
		createCmd.Env = append(createCmd.Env, fmt.Sprintf("PGPASSWORD=%s", password))
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
	restoreArgs := []string{"psql", "-d", restorePlan.Database}
	if user != "" {
		restoreArgs = append([]string{restoreArgs[0], "-U", user}, restoreArgs[1:]...)
	}
	if host != "" {
		restoreArgs = append(restoreArgs, "-h", host)
	}
	if port != "" {
		restoreArgs = append(restoreArgs, "-p", port)
	}
	if useSudo {
		restoreCmd = exec.Command("sudo", append([]string{"-u", "postgres"}, restoreArgs...)...)
	} else {
		restoreCmd = exec.Command(restoreArgs[0], restoreArgs[1:]...)
	}
	if restorePlan.RequiresAuth {
		restoreCmd.Env = append(restoreCmd.Env, fmt.Sprintf("PGPASSWORD=%s", password))
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

func writeDecompressedTempFile(dumpPath string, dumpInfo *validate.DumpInfo, ext string) (string, func(), bool, error) {
	reader, closeReader, usedFallback, err := validate.OpenDecompressedReaderBestEffort(dumpPath, dumpInfo)
	if err != nil {
		return "", func() {}, false, err
	}

	tmpFile, err := os.CreateTemp("", "mirrorvault_restore_*"+ext)
	if err != nil {
		_ = closeReader()
		return "", func() {}, usedFallback, err
	}

	if _, err := io.Copy(tmpFile, reader); err != nil {
		_ = tmpFile.Close()
		_ = closeReader()
		_ = os.Remove(tmpFile.Name())
		return "", func() {}, usedFallback, err
	}
	_ = tmpFile.Close()
	_ = closeReader()

	cleanup := func() {
		_ = os.Remove(tmpFile.Name())
	}
	return tmpFile.Name(), cleanup, usedFallback, nil
}
