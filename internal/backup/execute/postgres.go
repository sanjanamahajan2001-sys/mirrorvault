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

func postgresBackupFormat() string {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("MV_POSTGRES_BACKUP_FORMAT")))
	switch value {
	case "custom", "c":
		return "custom"
	case "directory", "dir", "d":
		return "directory"
	default:
		return "plain"
	}
}

func pgDumpBaseArgs(format string, outputPath string, includeNoOwner bool, includeNoAcl bool, verbose bool) []string {
	args := []string{"pg_dump"}
	switch format {
	case "custom":
		args = append(args, "-F", "c")
	case "directory":
		args = append(args, "-F", "d", "-f", outputPath)
	default:
		args = append(args, "-F", "p")
	}
	if includeNoOwner {
		args = append(args, "--no-owner")
	}
	if includeNoAcl {
		args = append(args, "--no-acl")
	}
	if verbose {
		args = append(args, "--verbose")
	}
	return args
}

func runPostgreSQL(
	engine plan.EnginePlan,
	creds *credentials.AuthContext,
	onProgress ProgressFunc,
) error {
	if err := requireCommand("pg_dump"); err != nil {
		return err
	}
	if err := requireCommand("psql"); err != nil {
		return err
	}

	baseDir := engine.OutputDir
	user := config.PostgresUser()
	host := config.PostgresHost()
	port := config.PostgresPort()
	useSudo := host == "" && user == "postgres"
	backupFormat := postgresBackupFormat()

	if err := ensureDir(baseDir); err != nil {
		return err
	}

	if engine.AllDatabases {
		return runPostgreSQLAllDatabases(engine, creds, onProgress)
	}

	for _, db := range engine.Databases {
		var err error
		// Generate output path based on format
		currentDate := time.Now().Format("2006-01-02")
		var outFile string
		switch backupFormat {
		case "custom":
			outFile = filepath.Join(baseDir, fmt.Sprintf("%s_%s.dump", db.Name, currentDate))
		case "directory":
			outFile = filepath.Join(baseDir, fmt.Sprintf("%s_%s", db.Name, currentDate))
		default:
			outFile = filepath.Join(baseDir, fmt.Sprintf("%s_%s.sql", db.Name, currentDate))
		}

		// ▶ running
		onProgress(
			engine.Engine,
			db.Name,
			outFile,
			0,
			"running",
			nil,
		)

		var cmd *exec.Cmd

		var f *os.File
		if backupFormat != "directory" {
			var err error
			f, err = createWritableFile(outFile)
			if err != nil {
				onProgress(engine.Engine, db.Name, "", 0, "failed", err)
				return err
			}
		} else {
			_ = os.RemoveAll(outFile)
		}

		baseArgs := pgDumpBaseArgs(backupFormat, outFile, false, false, false)
		if user != "" {
			baseArgs = append(baseArgs, "-U", user)
		}
		if host != "" {
			baseArgs = append(baseArgs, "-h", host)
		}
		if port != "" {
			baseArgs = append(baseArgs, "-p", port)
		}
		baseArgs = append(baseArgs, db.Name)

		if engine.RequiresAuth {
			pwd, ok := creds.Get("PostgreSQL")
			if !ok {
				_ = f.Close()
				err = fmt.Errorf("missing PostgreSQL credentials")
				onProgress(engine.Engine, db.Name, "", 0, "failed", err)
				return err
			}

			// pg_dump with password via PGPASSWORD environment variable
			if useSudo {
				cmd = exec.Command("sudo", append([]string{"-u", "postgres"}, baseArgs...)...)
			} else {
				cmd = exec.Command(baseArgs[0], baseArgs[1:]...)
			}
			// Set PGPASSWORD in the environment
			cmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", pwd))
		} else {
			// pg_dump without password
			if useSudo {
				cmd = exec.Command("sudo", append([]string{"-u", "postgres"}, baseArgs...)...)
			} else {
				cmd = exec.Command(baseArgs[0], baseArgs[1:]...)
			}
		}

		var stderr bytes.Buffer
		if f != nil {
			cmd.Stdout = f
		}
		cmd.Stderr = io.MultiWriter(&stderr, os.Stderr)

		err = cmd.Run()
		if err != nil {
			if f != nil {
				_ = f.Close()
			}
			_ = os.RemoveAll(outFile) // Remove incomplete output
			
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
				var f2 *os.File
				var err2 error
				if backupFormat != "directory" {
					f2, err2 = createWritableFile(outFile)
					if err2 != nil {
						errMsg := fmt.Errorf("pg_dump failed: %v\n%s\n(Fallback also failed: %v)", err, stderrStr, err2)
						onProgress(engine.Engine, db.Name, "", 0, "failed", errMsg)
						return errMsg
					}
				} else {
					_ = os.RemoveAll(outFile)
				}
				
				var cmd2 *exec.Cmd
				var stderr2 bytes.Buffer
				
				fallbackArgs := pgDumpBaseArgs(backupFormat, outFile, true, true, true)
				if user != "" {
					fallbackArgs = append(fallbackArgs, "-U", user)
				}
				if host != "" {
					fallbackArgs = append(fallbackArgs, "-h", host)
				}
				if port != "" {
					fallbackArgs = append(fallbackArgs, "-p", port)
				}
				fallbackArgs = append(fallbackArgs, db.Name)

				if useSudo {
					cmd2 = exec.Command("sudo", append([]string{"-u", "postgres"}, fallbackArgs...)...)
				} else {
					cmd2 = exec.Command(fallbackArgs[0], fallbackArgs[1:]...)
				}

				if engine.RequiresAuth {
					pwd, _ := creds.Get("PostgreSQL")
					cmd2.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", pwd))
				}
				
				if f2 != nil {
					cmd2.Stdout = f2
				}
				cmd2.Stderr = io.MultiWriter(&stderr2, os.Stderr)
				
				if err2 := cmd2.Run(); err2 != nil {
					if f2 != nil {
						_ = f2.Close()
					}
					_ = os.RemoveAll(outFile)
					
					// Both attempts failed - try dumping tables individually as last resort
					if err3 := tryDumpPostgresTablesIndividually(engine, creds, db.Name, outFile, onProgress); err3 != nil {
						errMsg := fmt.Errorf("pg_dump failed (tried multiple approaches):\n1. Standard: %v\n%s\n2. --no-owner --no-acl: %v\n%s\n3. Individual tables: %v", 
							err, stderrStr, err2, stderr2.String(), err3)
						onProgress(engine.Engine, db.Name, "", 0, "failed", errMsg)
						return errMsg
					}
					// Individual table dump succeeded
					if f2 != nil {
						_ = f2.Close()
					}
					f = f2
				} else {
					// Fallback succeeded
					if f2 != nil {
						_ = f2.Close()
					}
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

		if f != nil {
			_ = f.Close()
		}

		var size int64
		if backupFormat == "directory" {
			size, err = validateNonEmptyDir(outFile)
		} else {
			size, err = validateNonEmptyFile(outFile)
		}
		if err != nil {
			onProgress(engine.Engine, db.Name, "", 0, "failed", err)
			return err
		}
		if backupFormat == "plain" {
			if err := validateSQLDump(outFile); err != nil {
				onProgress(engine.Engine, db.Name, "", 0, "failed", err)
				return err
			}
		}

		compressedPath, compressedSize, err := applyBackupCompression(outFile, backupFormat == "directory")
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

func runPostgreSQLAllDatabases(
	engine plan.EnginePlan,
	creds *credentials.AuthContext,
	onProgress ProgressFunc,
) error {
	if err := requireCommand("pg_dump"); err != nil {
		return err
	}

	baseDir := engine.OutputDir
	user := config.PostgresUser()
	host := config.PostgresHost()
	port := config.PostgresPort()
	useSudo := host == "" && user == "postgres"

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

	if len(engine.Databases) == 0 {
		err := fmt.Errorf("no databases available for PostgreSQL backup")
		onProgress(engine.Engine, progressName, "", 0, "failed", err)
		return err
	}

	f, err := os.OpenFile(outFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		onProgress(engine.Engine, progressName, "", 0, "failed", err)
		return err
	}
	defer f.Close()

	for _, db := range engine.Databases {
		_, _ = f.WriteString(fmt.Sprintf("\n-- PostgreSQL dump for database: %s\n\n", db.Name))

		args := []string{"pg_dump", "-F", "p", "-C"}
		if user != "" {
			args = append(args, "-U", user)
		}
		if host != "" {
			args = append(args, "-h", host)
		}
		if port != "" {
			args = append(args, "-p", port)
		}
		args = append(args, db.Name)

		var cmd *exec.Cmd
		if useSudo {
			cmd = exec.Command("sudo", append([]string{"-u", "postgres"}, args...)...)
		} else {
			cmd = exec.Command(args[0], args[1:]...)
		}
		if engine.RequiresAuth {
			pwd, ok := creds.Get("PostgreSQL")
			if !ok {
				err := fmt.Errorf("missing PostgreSQL credentials")
				onProgress(engine.Engine, progressName, "", 0, "failed", err)
				return err
			}
			cmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", pwd))
		}

		var stderr bytes.Buffer
		cmd.Stdout = f
		cmd.Stderr = io.MultiWriter(&stderr, os.Stderr)

		if err := cmd.Run(); err != nil {
			_ = os.Remove(outFile)
			errMsg := fmt.Errorf("pg_dump failed for %s: %v", db.Name, err)
			if stderr.Len() > 0 {
				errMsg = fmt.Errorf("pg_dump failed for %s: %v\n%s", db.Name, err, stderr.String())
			}
			onProgress(engine.Engine, progressName, "", 0, "failed", errMsg)
			return errMsg
		}
	}

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

// tryDumpPostgresTablesIndividually attempts to dump tables one by one when full database dump fails
func tryDumpPostgresTablesIndividually(
	engine plan.EnginePlan,
	creds *credentials.AuthContext,
	dbName string,
	outFile string,
	onProgress ProgressFunc,
) error {
	user := config.PostgresUser()
	host := config.PostgresHost()
	port := config.PostgresPort()
	useSudo := host == "" && user == "postgres"

	// Get list of tables in the database
	var listCmd *exec.Cmd
	if engine.RequiresAuth {
		pwd, _ := creds.Get("PostgreSQL")
		listArgs := []string{"psql", "-t", "-c", "SELECT tablename FROM pg_tables WHERE schemaname='public';", dbName}
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
			listCmd = exec.Command("sudo", append([]string{"-u", "postgres"}, listArgs...)...)
		} else {
			listCmd = exec.Command(listArgs[0], listArgs[1:]...)
		}
		listCmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", pwd))
	} else {
		listArgs := []string{"psql", "-t", "-c", "SELECT tablename FROM pg_tables WHERE schemaname='public';", dbName}
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
			listCmd = exec.Command("sudo", append([]string{"-u", "postgres"}, listArgs...)...)
		} else {
			listCmd = exec.Command(listArgs[0], listArgs[1:]...)
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
	f.WriteString(fmt.Sprintf("-- PostgreSQL dump of database: %s\n", dbName))
	f.WriteString("-- Dumped table by table due to connection/data issues\n")
	f.WriteString(fmt.Sprintf("-- Generated: %s\n\n", time.Now().Format("2006-01-02 15:04:05")))
	
	// Try to dump each table individually
	failedTables := []string{}
	successCount := 0
	
	for _, tableName := range tableNames {
		var dumpCmd *exec.Cmd
		
		dumpArgs := []string{"pg_dump", "-F", "p", "--no-owner", "--no-acl", "-t", tableName}
		if user != "" {
			dumpArgs = append(dumpArgs, "-U", user)
		}
		if host != "" {
			dumpArgs = append(dumpArgs, "-h", host)
		}
		if port != "" {
			dumpArgs = append(dumpArgs, "-p", port)
		}
		dumpArgs = append(dumpArgs, dbName)

		if useSudo {
			dumpCmd = exec.Command("sudo", append([]string{"-u", "postgres"}, dumpArgs...)...)
		} else {
			dumpCmd = exec.Command(dumpArgs[0], dumpArgs[1:]...)
		}

		if engine.RequiresAuth {
			pwd, _ := creds.Get("PostgreSQL")
			dumpCmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", pwd))
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
