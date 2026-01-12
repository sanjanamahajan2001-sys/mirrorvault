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

func runPostgreSQL(
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

		// Create output file first
		f, err := os.Create(outFile)
		if err != nil {
			onProgress(engine.Engine, db.Name, "", 0, "failed", err)
			return err
		}

		var cmd *exec.Cmd

		if engine.RequiresAuth {
			pwd, ok := creds.Get("PostgreSQL")
			if !ok {
				_ = f.Close()
				err = fmt.Errorf("missing PostgreSQL credentials")
				onProgress(engine.Engine, db.Name, "", 0, "failed", err)
				return err
			}

			// pg_dump with password via PGPASSWORD environment variable
			// Use stdout redirection instead of -f flag for better compatibility with sudo
			cmd = exec.Command(
				"sudo",
				"-u", "postgres",
				"pg_dump",
				"-F", "p", // plain text format
				db.Name,
			)
			// Set PGPASSWORD in the environment
			cmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", pwd))
		} else {
			// pg_dump without password
			cmd = exec.Command(
				"sudo",
				"-u", "postgres",
				"pg_dump",
				"-F", "p", // plain text format
				db.Name,
			)
		}

		var stderr bytes.Buffer
		cmd.Stdout = f
		cmd.Stderr = io.MultiWriter(&stderr, os.Stderr)

		err = cmd.Run()
		if err != nil {
			_ = f.Close()
			_ = os.Remove(outFile) // Remove incomplete file
			
			stderrStr := stderr.String()
			// Check if it's a connection or data corruption error
			stderrBytes := []byte(stderrStr)
			isDataError := bytes.Contains(stderrBytes, []byte("corrupt")) ||
			              bytes.Contains(stderrBytes, []byte("invalid")) ||
			              bytes.Contains(stderrBytes, []byte("connection")) ||
			              bytes.Contains(stderrBytes, []byte("timeout")) ||
			              bytes.Contains(stderrBytes, []byte("lost connection"))
			
			// Try fallback with different flags
			if isDataError {
				// Retry with --no-owner and --no-acl flags (more compatible)
				f2, err2 := os.Create(outFile)
				if err2 != nil {
					errMsg := fmt.Errorf("pg_dump failed: %v\n%s\n(Fallback also failed: %v)", err, stderrStr, err2)
					onProgress(engine.Engine, db.Name, "", 0, "failed", errMsg)
					return errMsg
				}
				
				var cmd2 *exec.Cmd
				var stderr2 bytes.Buffer
				
				if engine.RequiresAuth {
					pwd, _ := creds.Get("PostgreSQL")
					cmd2 = exec.Command(
						"sudo",
						"-u", "postgres",
						"pg_dump",
						"-F", "p",
						"--no-owner",    // Skip ownership commands
						"--no-acl",      // Skip access privileges
						"--verbose",      // More detailed output for debugging
						db.Name,
					)
					cmd2.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", pwd))
				} else {
					cmd2 = exec.Command(
						"sudo",
						"-u", "postgres",
						"pg_dump",
						"-F", "p",
						"--no-owner",
						"--no-acl",
						"--verbose",
						db.Name,
					)
				}
				
				cmd2.Stdout = f2
				cmd2.Stderr = io.MultiWriter(&stderr2, os.Stderr)
				
				if err2 := cmd2.Run(); err2 != nil {
					_ = f2.Close()
					_ = os.Remove(outFile)
					
					// Both attempts failed - try dumping tables individually as last resort
					if err3 := tryDumpPostgresTablesIndividually(engine, creds, db.Name, outFile, onProgress); err3 != nil {
						errMsg := fmt.Errorf("pg_dump failed (tried multiple approaches):\n1. Standard: %v\n%s\n2. --no-owner --no-acl: %v\n%s\n3. Individual tables: %v", 
							err, stderrStr, err2, stderr2.String(), err3)
						onProgress(engine.Engine, db.Name, "", 0, "failed", errMsg)
						return errMsg
					}
					// Individual table dump succeeded
					_ = f2.Close()
					f = f2
				} else {
					// Fallback succeeded
					_ = f2.Close()
					f = f2
				}
			} else {
				// Not a data error, return original error
				errMsg := fmt.Errorf("pg_dump failed: %v", err)
				if stderr.Len() > 0 {
					errMsg = fmt.Errorf("pg_dump failed: %v\n%s", err, stderr.String())
				}
				onProgress(engine.Engine, db.Name, "", 0, "failed", errMsg)
				return errMsg
			}
		}

		_ = f.Close()

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

// tryDumpPostgresTablesIndividually attempts to dump tables one by one when full database dump fails
func tryDumpPostgresTablesIndividually(
	engine plan.EnginePlan,
	creds *credentials.AuthContext,
	dbName string,
	outFile string,
	onProgress ProgressFunc,
) error {
	// Get list of tables in the database
	var listCmd *exec.Cmd
	if engine.RequiresAuth {
		pwd, _ := creds.Get("PostgreSQL")
		listCmd = exec.Command("sudo", "-u", "postgres", "psql", "-t", "-c", 
			fmt.Sprintf("SELECT tablename FROM pg_tables WHERE schemaname='public';"), dbName)
		listCmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", pwd))
	} else {
		listCmd = exec.Command("sudo", "-u", "postgres", "psql", "-t", "-c", 
			fmt.Sprintf("SELECT tablename FROM pg_tables WHERE schemaname='public';"), dbName)
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
	f.WriteString(fmt.Sprintf("-- PostgreSQL dump of database: %s\n", dbName))
	f.WriteString("-- Dumped table by table due to connection/data issues\n")
	f.WriteString(fmt.Sprintf("-- Generated: %s\n\n", time.Now().Format("2006-01-02 15:04:05")))
	
	// Try to dump each table individually
	failedTables := []string{}
	successCount := 0
	
	for _, tableName := range tableNames {
		var dumpCmd *exec.Cmd
		
		if engine.RequiresAuth {
			pwd, _ := creds.Get("PostgreSQL")
			dumpCmd = exec.Command(
				"sudo",
				"-u", "postgres",
				"pg_dump",
				"-F", "p",
				"--no-owner",
				"--no-acl",
				"-t", tableName,
				dbName,
			)
			dumpCmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", pwd))
		} else {
			dumpCmd = exec.Command(
				"sudo",
				"-u", "postgres",
				"pg_dump",
				"-F", "p",
				"--no-owner",
				"--no-acl",
				"-t", tableName,
				dbName,
			)
		}
		
		var tableStderr bytes.Buffer
		dumpCmd.Stdout = f
		dumpCmd.Stderr = io.MultiWriter(&tableStderr, os.Stderr)
		
		if err := dumpCmd.Run(); err != nil {
			// This table failed - log it and continue with others
			failedTables = append(failedTables, fmt.Sprintf("%s: %v", tableName, err))
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
		return nil
	}
	
	return nil
}
