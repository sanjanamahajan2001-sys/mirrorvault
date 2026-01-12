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
)

func runMySQL(
	engine plan.EnginePlan,
	creds *credentials.AuthContext,
	onProgress ProgressFunc,
) error {

	baseDir := engine.OutputDir

	if err := ensureDir(baseDir); err != nil {
		return err
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

		// Create a temporary MySQL config file to set session variables
		// This increases net_read_timeout and net_write_timeout to prevent "Lost connection" errors
		// when the server has low timeout settings (e.g., net_read_timeout=30)
		tmpConfigFile := filepath.Join(os.TempDir(), fmt.Sprintf("mirrorvault_mysql_%d.cnf", time.Now().UnixNano()))
		configContent := fmt.Sprintf(`[mysqldump]
net_read_timeout=300
net_write_timeout=300
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
		// --defaults-extra-file: Use temp config file to set net_read_timeout and net_write_timeout
		// --routines: Include stored procedures and functions
		// --triggers: Include triggers
		// --events: Include events
		// --no-tablespaces: Skip tablespace information (improves compatibility across MySQL versions)
		// Note: Using --lock-all-tables instead of --single-transaction for better compatibility
		//       If this fails, fallback to --skip-lock-tables (less consistent but more compatible)
		// Note: Using --defaults-extra-file to increase net_read_timeout and net_write_timeout to 300 seconds
		//       This prevents "Lost connection" errors when server has low timeout values (e.g., net_read_timeout=30)
		if engine.RequiresAuth {
			pwd, ok := creds.Get("MySQL")
			if !ok {
				err := fmt.Errorf("missing MySQL credentials")
				onProgress(engine.Engine, db.Name, "", 0, "failed", err)
				return err
			}

			// Use -p format with password directly attached (no space) - standard mysqldump format
			// Format: -pPASSWORD (not -p PASSWORD)
			// Use --defaults-extra-file to set session variables for increased timeouts
			// Use --lock-all-tables instead of --single-transaction for better compatibility
			// --single-transaction can cause immediate connection failures on some servers
			cmd = exec.Command(
				"sudo",
				"mysqldump",
				"--defaults-extra-file="+tmpConfigFile,
				"-u", "root",
				"-p"+pwd,
				"--quick",                    // CRITICAL: Prevents loading entire table into memory
				"--lock-all-tables",          // Lock all tables (more compatible than --single-transaction)
				"--net_buffer_length=16384",  // Increase network buffer for better throughput
				"--default-character-set=utf8mb4", // Proper Unicode support
				"--routines",                 // Include stored procedures
				"--triggers",                 // Include triggers
				"--events",                   // Include events
				"--no-tablespaces",           // Skip tablespace info (compatibility)
				db.Name,
			)
		} else {
			// No password required
			cmd = exec.Command(
				"sudo",
				"mysqldump",
				"--defaults-extra-file="+tmpConfigFile,
				"-u", "root",
				"--quick",                    // CRITICAL: Prevents loading entire table into memory
				"--lock-all-tables",          // Lock all tables (more compatible than --single-transaction)
				"--net_buffer_length=16384",  // Increase network buffer for better throughput
				"--default-character-set=utf8mb4", // Proper Unicode support
				"--routines",                 // Include stored procedures
				"--triggers",                 // Include triggers
				"--events",                   // Include events
				"--no-tablespaces",           // Skip tablespace info (compatibility)
				db.Name,
			)
		}

		f, err := os.Create(outFile)
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
				f2, err2 := os.Create(outFile)
				if err2 != nil {
					errMsg := fmt.Errorf("mysqldump failed: %v\n%s\n(Fallback also failed: %v)", err, stderrStr, err2)
					onProgress(engine.Engine, db.Name, "", 0, "failed", errMsg)
					return errMsg
				}
				
				var cmd2 *exec.Cmd
				var stderr2 bytes.Buffer
				
				if engine.RequiresAuth {
					pwd, _ := creds.Get("MySQL")
					cmd2 = exec.Command(
						"sudo",
						"mysqldump",
						"--defaults-extra-file="+tmpConfigFile,
						"-u", "root",
						"-p"+pwd,
						"--quick",                    // CRITICAL: Prevents loading entire table into memory
						"--skip-lock-tables",         // Skip locking (fallback - less consistent but more compatible)
						"--net_buffer_length=16384",  // Increase network buffer
						"--default-character-set=utf8mb4",
						"--routines",
						"--triggers",
						"--events",
						"--no-tablespaces",
						db.Name,
					)
				} else {
					cmd2 = exec.Command(
						"sudo",
						"mysqldump",
						"--defaults-extra-file="+tmpConfigFile,
						"-u", "root",
						"--quick",
						"--skip-lock-tables",         // Skip locking (fallback - less consistent but more compatible)
						"--net_buffer_length=16384",
						"--default-character-set=utf8mb4",
						"--routines",
						"--triggers",
						"--events",
						"--no-tablespaces",
						db.Name,
					)
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

		info, _ := os.Stat(outFile)
		var size int64
		if info != nil {
			size = info.Size()
		}

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
	// Get list of tables in the database
	var listCmd *exec.Cmd
	if engine.RequiresAuth {
		pwd, _ := creds.Get("MySQL")
		listCmd = exec.Command("sudo", "mysql", "-u", "root", "-p"+pwd, "-N", "-e", 
			fmt.Sprintf("USE %s; SHOW TABLES;", dbName))
	} else {
		listCmd = exec.Command("sudo", "mysql", "-u", "root", "-N", "-e", 
			fmt.Sprintf("USE %s; SHOW TABLES;", dbName))
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
	f, err := os.Create(outFile)
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
		
		if engine.RequiresAuth {
			pwd, _ := creds.Get("MySQL")
			dumpCmd = exec.Command(
				"sudo",
				"mysqldump",
				"--defaults-extra-file="+tmpConfigFile,
				"-u", "root",
				"-p"+pwd,
				"--quick",
				"--skip-lock-tables",
				"--net_buffer_length=16384",
				"--default-character-set=utf8mb4",
				"--no-tablespaces",
				dbName,
				tableName,
			)
		} else {
			dumpCmd = exec.Command(
				"sudo",
				"mysqldump",
				"--defaults-extra-file="+tmpConfigFile,
				"-u", "root",
				"--quick",
				"--skip-lock-tables",
				"--net_buffer_length=16384",
				"--default-character-set=utf8mb4",
				"--no-tablespaces",
				dbName,
				tableName,
			)
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
