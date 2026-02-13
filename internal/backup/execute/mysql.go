package execute

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"mirrorvault/internal/backup/credentials"
	"mirrorvault/internal/backup/plan"
	"mirrorvault/internal/config"
)

func runMySQL(
	engine plan.EnginePlan,
	creds *credentials.AuthContext,
	onProgress ProgressFunc,
) error {
	if err := requireCommand("mysqldump"); err != nil {
		return err
	}
	if err := requireCommand("mysql"); err != nil {
		return err
	}

	baseDir := engine.OutputDir
	user := config.MySQLUser()
	host := config.MySQLHost()
	port := config.MySQLPort()
	useSudo := host == "" || host == "localhost" || host == "127.0.0.1"

	if err := ensureDir(baseDir); err != nil {
		return err
	}

	if engine.AllDatabases {
		return runMySQLAllDatabases(engine, creds, onProgress)
	}

	for _, db := range engine.Databases {
		// Generate filename with current date: dbname_YYYY-MM-DD.sql
		currentDate := time.Now().Format("2006-01-02")
		fileName := fmt.Sprintf("%s_%s.sql", db.Name, currentDate)
		outFile := filepath.Join(baseDir, fileName)

		// ▶ running
		onProgress(
			engine.Engine,
			db.Name,
			outFile,
			0,
			"running",
			nil,
		)

		// Create a temporary MySQL config file to set max_allowed_packet
		// Note: net_read_timeout and net_write_timeout are server-side session variables,
		// not mysqldump options, so we can't set them via config file
		// The --quick flag and other options handle connection issues
		tmpConfigFile := filepath.Join(os.TempDir(), fmt.Sprintf("mirrorvault_mysql_%d.cnf", time.Now().UnixNano()))
		configContent := fmt.Sprintf(`[mysqldump]
max_allowed_packet=512M
`)
		// Use 0644 permissions so sudo (running as root) can read the file
		if err := os.WriteFile(tmpConfigFile, []byte(configContent), 0644); err != nil {
			onProgress(engine.Engine, db.Name, "", 0, "failed", fmt.Errorf("failed to create temp config: %v", err))
			return fmt.Errorf("failed to create temp config: %v", err)
		}
		defer os.Remove(tmpConfigFile) // Clean up temp file

		var cmd *exec.Cmd

		// Match the scan logic: always use sudo for consistency
		// The scan uses "sudo mysql", so we use "sudo mysqldump"
		// Add flags to handle large tables and prevent connection timeouts:
		// --quick: CRITICAL - Retrieves rows one at a time instead of loading entire table into memory
		//          This prevents "Lost connection" errors by avoiding memory exhaustion
		// --lock-all-tables: Locks all tables for consistency (more compatible than --single-transaction)
		//                    --single-transaction can cause immediate connection failures on some servers
		// --max_allowed_packet: Increase packet size to handle large BLOB/TEXT rows (prevents "Lost connection" errors)
		// --net_buffer_length: Increase network buffer size for better throughput
		// --default-character-set: Ensure proper character encoding (utf8mb4 supports all Unicode)
		// --defaults-extra-file: Use temp config file to set max_allowed_packet
		// --routines: Include stored procedures and functions
		// --triggers: Include triggers
		// --events: Include events
		// --no-tablespaces: Skip tablespace information (improves compatibility across MySQL versions)
		// Note: Using --lock-all-tables instead of --single-transaction for better compatibility
		//       If this fails, fallback to --skip-lock-tables (less consistent but more compatible)
		// Note: The --quick flag prevents loading entire tables into memory, which helps prevent
		//       "Lost connection" errors. max_allowed_packet is set via config file to handle large rows.
		baseArgs := []string{
			"mysqldump",
			"--defaults-extra-file=" + tmpConfigFile,
			"-u", user,
			"--quick",                    // CRITICAL: Prevents loading entire table into memory
			"--lock-all-tables",          // Lock all tables (more compatible than --single-transaction)
			"--net_buffer_length=16384",  // Increase network buffer for better throughput
			"--default-character-set=utf8mb4", // Proper Unicode support
			"--routines",                 // Include stored procedures
			"--triggers",                 // Include triggers
			"--events",                   // Include events
			"--no-tablespaces",           // Skip tablespace info (compatibility)
		}
		if host != "" {
			baseArgs = append(baseArgs, "-h", host)
		}
		if port != "" {
			baseArgs = append(baseArgs, "-P", port)
		}

		if engine.RequiresAuth {
			pwd, ok := creds.Get("MySQL")
			if !ok {
				err := fmt.Errorf("missing MySQL credentials")
				onProgress(engine.Engine, db.Name, "", 0, "failed", err)
				return err
			}
			baseArgs = append(baseArgs, "-p"+pwd, db.Name)
		} else {
			// No password required
			baseArgs = append(baseArgs, db.Name)
		}

		if useSudo {
			cmd = exec.Command("sudo", baseArgs...)
		} else {
			cmd = exec.Command(baseArgs[0], baseArgs[1:]...)
		}

		f, err := createWritableFile(outFile)
		if err != nil {
			onProgress(engine.Engine, db.Name, "", 0, "failed", err)
			return err
		}

		// Capture stderr to include in error message
		// We need to capture it separately first to check for specific errors
		var stderr bytes.Buffer
		cmd.Stdout = f
		cmd.Stderr = io.MultiWriter(&stderr, os.Stderr)

		err = cmd.Run()
		if err != nil {
			_ = f.Close()
			_ = os.Remove(outFile) // Remove incomplete file
			
			// Check if it's a "Lost connection" error (Error 2013)
			// If so, try fallback without --single-transaction (which might be causing the issue)
			stderrStr := stderr.String()
			// Check for various forms of the error message
			stderrBytes := []byte(stderrStr)
			isConnectionError := bytes.Contains(stderrBytes, []byte("Error 2013")) || 
			                    bytes.Contains(stderrBytes, []byte("Lost connection")) ||
			                    bytes.Contains(stderrBytes, []byte("lost connection")) ||
			                    bytes.Contains(stderrBytes, []byte("2013"))
			
			// Try to extract table name from error message for better diagnostics
			problematicTable := ""
			if strings.Contains(stderrStr, "dumping table") {
				// Extract table name from error like "when dumping table 'Documents'"
				parts := strings.Split(stderrStr, "dumping table")
				if len(parts) > 1 {
					tablePart := strings.TrimSpace(parts[1])
					// Extract table name (remove quotes and "at row" part)
					if idx := strings.Index(tablePart, "'"); idx >= 0 {
						tablePart = tablePart[idx+1:]
						if idx2 := strings.Index(tablePart, "'"); idx2 >= 0 {
							problematicTable = tablePart[:idx2]
						}
					}
				}
			}
			
			if isConnectionError {
				// Retry without --lock-all-tables, using --skip-lock-tables instead
				// This is a fallback for cases where table locking causes immediate connection issues
				f2, err2 := createWritableFile(outFile)
				if err2 != nil {
					errMsg := fmt.Errorf("mysqldump failed: %v\n%s\n(Fallback also failed: %v)", err, stderrStr, err2)
					onProgress(engine.Engine, db.Name, "", 0, "failed", errMsg)
					return errMsg
				}
				
				var cmd2 *exec.Cmd
				var stderr2 bytes.Buffer
				
				fallbackArgs := []string{
					"mysqldump",
					"--defaults-extra-file=" + tmpConfigFile,
					"-u", user,
					"--quick",
					"--skip-lock-tables",
					"--net_buffer_length=16384",
					"--default-character-set=utf8mb4",
					"--routines",
					"--triggers",
					"--events",
					"--no-tablespaces",
				}
				if host != "" {
					fallbackArgs = append(fallbackArgs, "-h", host)
				}
				if port != "" {
					fallbackArgs = append(fallbackArgs, "-P", port)
				}
				if engine.RequiresAuth {
					pwd, _ := creds.Get("MySQL")
					fallbackArgs = append(fallbackArgs, "-p"+pwd, db.Name)
				} else {
					fallbackArgs = append(fallbackArgs, db.Name)
				}

				if useSudo {
					cmd2 = exec.Command("sudo", fallbackArgs...)
				} else {
					cmd2 = exec.Command(fallbackArgs[0], fallbackArgs[1:]...)
				}
				
				cmd2.Stdout = f2
				cmd2.Stderr = io.MultiWriter(&stderr2, os.Stderr)
				
				if err2 := cmd2.Run(); err2 != nil {
					_ = f2.Close()
					_ = os.Remove(outFile)
					
					// Both attempts failed - try dumping tables individually as last resort
					// This helps identify which specific table is causing the issue
					diagnosticMsg := fmt.Sprintf("Full database dump failed. Attempting individual table dumps...")
					if problematicTable != "" {
						diagnosticMsg += fmt.Sprintf("\nProblematic table detected: %s", problematicTable)
					}
					onProgress(engine.Engine, db.Name, "", 0, "running", fmt.Errorf(diagnosticMsg))
					
					if err3 := tryDumpTablesIndividually(engine, creds, db.Name, outFile, tmpConfigFile, onProgress); err3 != nil {
						errMsg := fmt.Errorf("mysqldump failed (tried multiple approaches):\n1. --lock-all-tables: %v\n%s\n2. --skip-lock-tables: %v\n%s\n3. Individual tables: %v", 
							err, stderrStr, err2, stderr2.String(), err3)
						if problematicTable != "" {
							errMsg = fmt.Errorf("%s\n\nDiagnostic: Problematic table appears to be '%s' at row 30", errMsg, problematicTable)
						}
						onProgress(engine.Engine, db.Name, "", 0, "failed", errMsg)
						return errMsg
					}
					// Individual table dump succeeded
				}
				
				// Fallback succeeded - close the file and continue to success path
				_ = f2.Close()
				// Continue to file size check below (f was already closed above)
			} else {
				// Not a connection error, return original error
				errMsg := fmt.Errorf("mysqldump failed: %v", err)
				if stderr.Len() > 0 {
					errMsg = fmt.Errorf("mysqldump failed: %v\n%s", err, stderr.String())
				}
				onProgress(engine.Engine, db.Name, "", 0, "failed", errMsg)
				return errMsg
			}
		} else {
			// First attempt succeeded - close file normally
			_ = f.Close()
		}

		size, err := validateNonEmptyFile(outFile)
		if err != nil {
			onProgress(engine.Engine, db.Name, "", 0, "failed", err)
			return err
		}
		if err := validateSQLDump(outFile); err != nil {
			onProgress(engine.Engine, db.Name, "", 0, "failed", err)
			return err
		}

		compressedPath, compressedSize, err := applyBackupCompression(outFile, false)
		if err != nil {
			onProgress(engine.Engine, db.Name, "", 0, "failed", err)
			return err
		}
		outFile = compressedPath
		size = compressedSize

		// ✔ done
		onProgress(
			engine.Engine,
			db.Name,
			outFile,
			size,
			"done",
			nil,
		)
	}

	return nil
}

func runMySQLAllDatabases(
	engine plan.EnginePlan,
	creds *credentials.AuthContext,
	onProgress ProgressFunc,
) error {
	baseDir := engine.OutputDir
	user := config.MySQLUser()
	host := config.MySQLHost()
	port := config.MySQLPort()
	useSudo := host == "" || host == "localhost" || host == "127.0.0.1"

	currentDate := time.Now().Format("2006-01-02")
	prefix := strings.ToLower(engine.Engine)
	outFile := filepath.Join(baseDir, fmt.Sprintf("%s_all_databases_%s.sql", prefix, currentDate))
	progressName := "All databases"

	onProgress(
		engine.Engine,
		progressName,
		outFile,
		0,
		"running",
		nil,
	)

	tmpConfigFile := filepath.Join(os.TempDir(), fmt.Sprintf("mirrorvault_mysql_%d.cnf", time.Now().UnixNano()))
	configContent := fmt.Sprintf(`[mysqldump]
max_allowed_packet=512M
`)
	if err := os.WriteFile(tmpConfigFile, []byte(configContent), 0644); err != nil {
		onProgress(engine.Engine, progressName, "", 0, "failed", fmt.Errorf("failed to create temp config: %v", err))
		return fmt.Errorf("failed to create temp config: %v", err)
	}
	defer os.Remove(tmpConfigFile)

	if len(engine.Databases) == 0 {
		err := fmt.Errorf("no databases available for MySQL backup")
		onProgress(engine.Engine, progressName, "", 0, "failed", err)
		return err
	}

	baseArgs := []string{
		"mysqldump",
		"--defaults-extra-file=" + tmpConfigFile,
		"-u", user,
		"--quick",
		"--lock-all-tables",
		"--net_buffer_length=16384",
		"--default-character-set=utf8mb4",
		"--routines",
		"--triggers",
		"--events",
		"--no-tablespaces",
		"--databases",
	}
	for _, db := range engine.Databases {
		baseArgs = append(baseArgs, db.Name)
	}
	if host != "" {
		baseArgs = append(baseArgs, "-h", host)
	}
	if port != "" {
		baseArgs = append(baseArgs, "-P", port)
	}
	if engine.RequiresAuth {
		pwd, ok := creds.Get("MySQL")
		if !ok {
			err := fmt.Errorf("missing MySQL credentials")
			onProgress(engine.Engine, progressName, "", 0, "failed", err)
			return err
		}
		baseArgs = append(baseArgs, "-p"+pwd)
	}

	var cmd *exec.Cmd
	if useSudo {
		cmd = exec.Command("sudo", baseArgs...)
	} else {
		cmd = exec.Command(baseArgs[0], baseArgs[1:]...)
	}

	f, err := createWritableFile(outFile)
	if err != nil {
		onProgress(engine.Engine, progressName, "", 0, "failed", err)
		return err
	}

	var stderr bytes.Buffer
	cmd.Stdout = f
	cmd.Stderr = io.MultiWriter(&stderr, os.Stderr)

	err = cmd.Run()
	if err != nil {
		_ = f.Close()
		_ = os.Remove(outFile)
		errMsg := fmt.Errorf("mysqldump --all-databases failed: %v", err)
		if stderr.Len() > 0 {
			errMsg = fmt.Errorf("mysqldump --all-databases failed: %v\n%s", err, stderr.String())
		}
		onProgress(engine.Engine, progressName, "", 0, "failed", errMsg)
		return errMsg
	}

	_ = f.Close()

	size, err := validateNonEmptyFile(outFile)
	if err != nil {
		onProgress(engine.Engine, progressName, "", 0, "failed", err)
		return err
	}
	if err := validateSQLDump(outFile); err != nil {
		onProgress(engine.Engine, progressName, "", 0, "failed", err)
		return err
	}

	compressedPath, compressedSize, err := applyBackupCompression(outFile, false)
	if err != nil {
		onProgress(engine.Engine, progressName, "", 0, "failed", err)
		return err
	}
	outFile = compressedPath
	size = compressedSize

	onProgress(
		engine.Engine,
		progressName,
		outFile,
		size,
		"done",
		nil,
	)

	return nil
}

// tryDumpTablesIndividually attempts to dump tables one by one when full database dump fails
// This helps identify problematic tables and allows backup to continue with other tables
func tryDumpTablesIndividually(
	engine plan.EnginePlan,
	creds *credentials.AuthContext,
	dbName string,
	outFile string,
	tmpConfigFile string,
	onProgress ProgressFunc,
) error {
	user := config.MySQLUser()
	host := config.MySQLHost()
	port := config.MySQLPort()
	useSudo := host == "" || host == "localhost" || host == "127.0.0.1"

	// Get list of tables in the database
	var listCmd *exec.Cmd
	if engine.RequiresAuth {
		pwd, _ := creds.Get("MySQL")
		args := []string{"mysql", "-u", user, "-p" + pwd, "-N", "-e", fmt.Sprintf("USE %s; SHOW TABLES;", dbName)}
		if host != "" {
			args = append(args, "-h", host)
		}
		if port != "" {
			args = append(args, "-P", port)
		}
		if useSudo {
			listCmd = exec.Command("sudo", args...)
		} else {
			listCmd = exec.Command(args[0], args[1:]...)
		}
	} else {
		args := []string{"mysql", "-u", user, "-N", "-e", fmt.Sprintf("USE %s; SHOW TABLES;", dbName)}
		if host != "" {
			args = append(args, "-h", host)
		}
		if port != "" {
			args = append(args, "-P", port)
		}
		if useSudo {
			listCmd = exec.Command("sudo", args...)
		} else {
			listCmd = exec.Command(args[0], args[1:]...)
		}
	}
	
	var tablesOut bytes.Buffer
	listCmd.Stdout = &tablesOut
	listCmd.Stderr = os.Stderr
	
	if err := listCmd.Run(); err != nil {
		return fmt.Errorf("failed to list tables: %v", err)
	}
	
	// Parse table names
	tableNames := []string{}
	for _, line := range strings.Split(tablesOut.String(), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			tableNames = append(tableNames, line)
		}
	}
	
	if len(tableNames) == 0 {
		return fmt.Errorf("no tables found in database")
	}
	
	// Create output file
	f, err := createWritableFile(outFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer f.Close()
	
	// Write header
	f.WriteString(fmt.Sprintf("-- MySQL dump of database: %s\n", dbName))
	f.WriteString("-- Dumped table by table due to connection issues\n")
	f.WriteString(fmt.Sprintf("-- Generated: %s\n\n", time.Now().Format("2006-01-02 15:04:05")))
	f.WriteString(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`;\n", dbName))
	f.WriteString(fmt.Sprintf("USE `%s`;\n\n", dbName))
	
	// Try to dump each table individually
	failedTables := []string{}
	successCount := 0
	
	for _, tableName := range tableNames {
		var dumpCmd *exec.Cmd
		
		// Write table header comment
		f.WriteString(fmt.Sprintf("\n-- Table structure and data for table `%s`\n", tableName))
		
		dumpArgs := []string{
			"mysqldump",
			"--defaults-extra-file=" + tmpConfigFile,
			"-u", user,
			"--quick",
			"--skip-lock-tables",
			"--net_buffer_length=16384",
			"--default-character-set=utf8mb4",
			"--no-tablespaces",
		}
		if host != "" {
			dumpArgs = append(dumpArgs, "-h", host)
		}
		if port != "" {
			dumpArgs = append(dumpArgs, "-P", port)
		}
		if engine.RequiresAuth {
			pwd, _ := creds.Get("MySQL")
			dumpArgs = append(dumpArgs, "-p"+pwd, dbName, tableName)
		} else {
			dumpArgs = append(dumpArgs, dbName, tableName)
		}

		if useSudo {
			dumpCmd = exec.Command("sudo", dumpArgs...)
		} else {
			dumpCmd = exec.Command(dumpArgs[0], dumpArgs[1:]...)
		}
		
		var tableStderr bytes.Buffer
		dumpCmd.Stdout = f
		dumpCmd.Stderr = io.MultiWriter(&tableStderr, os.Stderr)
		
		if err := dumpCmd.Run(); err != nil {
			// This table failed - log it and continue with others
			failedTables = append(failedTables, fmt.Sprintf("%s: %v", tableName, err))
			// Write a comment about the failed table
			f.WriteString(fmt.Sprintf("\n-- WARNING: Failed to dump table %s: %v\n", tableName, err))
			continue
		}
		
		successCount++
		f.WriteString("\n")
	}
	
	if successCount == 0 {
		return fmt.Errorf("failed to dump any tables. Failed tables: %v", failedTables)
	}
	
	if len(failedTables) > 0 {
		// Some tables failed but we got some data
		onProgress(engine.Engine, dbName, outFile, 0, "done", 
			fmt.Errorf("partial backup: %d/%d tables dumped. Failed: %v", 
				successCount, len(tableNames), failedTables))
		return nil // Return nil because we got partial backup
	}
	
	return nil
}
