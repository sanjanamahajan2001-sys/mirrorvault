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

func sqliteBackupMode() string {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("MV_SQLITE_BACKUP_MODE")))
	if value == "backup" {
		return "backup"
	}
	return "dump"
}

func runSQLite(
	engine plan.EnginePlan,
	creds *credentials.AuthContext,
	onProgress ProgressFunc,
) error {
	if err := requireCommand("sqlite3"); err != nil {
		return err
	}

	baseDir := engine.OutputDir

	if err := ensureDir(baseDir); err != nil {
		return err
	}

	if engine.AllDatabases {
		return runSQLiteAllDatabases(engine, onProgress)
	}

	for _, db := range engine.Databases {
		// For SQLite, db.Name is the full path to the .db file
		dbPath := db.Name
		// Use the full path as the database identifier to match ExecState
		dbIdentifier := dbPath
		// Extract base name for filename generation
		dbBaseName := strings.TrimSuffix(filepath.Base(dbPath), filepath.Ext(dbPath))

		backupMode := sqliteBackupMode()
		// Generate filename with current date
		currentDate := time.Now().Format("2006-01-02")
		ext := ".sql"
		if backupMode == "backup" {
			ext = ".db"
		}
		fileName := fmt.Sprintf("%s_%s%s", dbBaseName, currentDate, ext)
		outFile := filepath.Join(baseDir, fileName)

		// ▶ running
		onProgress(
			engine.Engine,
			dbIdentifier, // Use full path to match ExecState
			outFile,
			0,
			"running",
			nil,
		)

		// Check if database file exists and is readable
		fileInfo, err := os.Stat(dbPath)
		if os.IsNotExist(err) {
			err := fmt.Errorf("database file does not exist: %s", dbPath)
			onProgress(engine.Engine, dbIdentifier, "", 0, "failed", err)
			return err
		}
		if err != nil {
			err := fmt.Errorf("cannot access database file: %s: %v", dbPath, err)
			onProgress(engine.Engine, dbIdentifier, "", 0, "failed", err)
			return err
		}

		// Check if file is readable
		if fileInfo.Mode().Perm()&0444 == 0 {
			err := fmt.Errorf("database file is not readable: %s", dbPath)
			onProgress(engine.Engine, dbIdentifier, "", 0, "failed", err)
			return err
		}

		if backupMode == "backup" {
			var stderr bytes.Buffer
			cmd := exec.Command("sqlite3", dbPath, fmt.Sprintf(".backup '%s'", outFile))
			cmd.Stderr = io.MultiWriter(&stderr, os.Stderr)

			if err := cmd.Run(); err != nil {
				stderrStr := stderr.String()
				errMsg := fmt.Errorf("sqlite3 backup failed: %v", err)
				if stderrStr != "" {
					errMsg = fmt.Errorf("sqlite3 backup failed: %v\n%s", err, stderrStr)
				}
				onProgress(engine.Engine, dbIdentifier, "", 0, "failed", errMsg)
				return errMsg
			}

			size, err := validateNonEmptyFile(outFile)
			if err != nil {
				onProgress(engine.Engine, dbIdentifier, "", 0, "failed", err)
				return err
			}
			compressedPath, compressedSize, err := applyBackupCompression(outFile, false)
			if err != nil {
				onProgress(engine.Engine, dbIdentifier, "", 0, "failed", err)
				return err
			}
			outFile = compressedPath
			size = compressedSize

			onProgress(
				engine.Engine,
				dbIdentifier,
				outFile,
				size,
				"done",
				nil,
			)
			continue
		}

		// Create output file first
		f, err := createWritableFile(outFile)
		if err != nil {
			onProgress(engine.Engine, dbBaseName, "", 0, "failed", err)
			return err
		}

		// SQLite backup using .dump command with manual timeout handling
		// Use Start() and Wait() separately for better process control
		var stderr bytes.Buffer
		cmd := exec.Command("sqlite3", "-batch", "-readonly", dbPath, ".dump")
		cmd.Stdout = f
		cmd.Stderr = io.MultiWriter(&stderr, os.Stderr)

		// Start the command
		if err := cmd.Start(); err != nil {
			_ = f.Close()
			_ = os.Remove(outFile)
			errMsg := fmt.Errorf("failed to start sqlite3: %v", err)
			onProgress(engine.Engine, dbIdentifier, "", 0, "failed", errMsg)
			return errMsg
		}

		// Wait for completion with timeout
		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
		}()

		// Wait for either completion or timeout (30 seconds for large databases)
		select {
		case err := <-done:
			_ = f.Close()
			if err != nil {
				// Remove incomplete file on error
				_ = os.Remove(outFile)
				
				stderrStr := stderr.String()
				// Check if it's a database lock or corruption error
				isLockError := strings.Contains(stderrStr, "locked") ||
				               strings.Contains(stderrStr, "database is locked") ||
				               strings.Contains(stderrStr, "busy")
				isCorruptError := strings.Contains(stderrStr, "corrupt") ||
				                  strings.Contains(stderrStr, "malformed")
				
				// Try fallback with different approach if it's a lock error
				if isLockError {
					// Retry with a small delay and different approach
					time.Sleep(1 * time.Second)
					f2, err2 := os.Create(outFile)
					if err2 != nil {
						errMsg := fmt.Errorf("sqlite3 backup failed: %v\n%s\n(Retry also failed: %v)", err, stderrStr, err2)
						onProgress(engine.Engine, dbIdentifier, "", 0, "failed", errMsg)
						return errMsg
					}
					
					var cmd2 *exec.Cmd
					var stderr2 bytes.Buffer
					// Try without readonly flag (might help with some lock issues)
					cmd2 = exec.Command("sqlite3", "-batch", dbPath, ".dump")
					cmd2.Stdout = f2
					cmd2.Stderr = io.MultiWriter(&stderr2, os.Stderr)
					
					if err2 := cmd2.Run(); err2 != nil {
						_ = f2.Close()
						_ = os.Remove(outFile)
						errMsg := fmt.Errorf("sqlite3 backup failed (tried readonly and normal mode):\n1. Readonly: %v\n%s\n2. Normal: %v\n%s", 
							err, stderrStr, err2, stderr2.String())
						onProgress(engine.Engine, dbIdentifier, "", 0, "failed", errMsg)
						return errMsg
					}
					
					_ = f2.Close()
					f = f2
				} else if isCorruptError {
					errMsg := fmt.Errorf("sqlite3 backup failed: database appears to be corrupted: %v\n%s", err, stderrStr)
					onProgress(engine.Engine, dbIdentifier, "", 0, "failed", errMsg)
					return errMsg
				} else {
					errMsg := fmt.Errorf("sqlite3 backup failed: %v", err)
					if stderr.Len() > 0 {
						errMsg = fmt.Errorf("sqlite3 backup failed: %v\n%s", err, stderr.String())
					}
					onProgress(engine.Engine, dbIdentifier, "", 0, "failed", errMsg)
					return errMsg
				}
			}
			// Success - get file size and report done
			size, err := validateNonEmptyFile(outFile)
			if err != nil {
				onProgress(engine.Engine, dbIdentifier, "", 0, "failed", err)
				return err
			}
			if err := validateSQLDump(outFile); err != nil {
				onProgress(engine.Engine, dbIdentifier, "", 0, "failed", err)
				return err
			}
			compressedPath, compressedSize, err := applyBackupCompression(outFile, false)
			if err != nil {
				onProgress(engine.Engine, dbIdentifier, "", 0, "failed", err)
				return err
			}
			outFile = compressedPath
			size = compressedSize
			onProgress(
				engine.Engine,
				dbIdentifier,
				outFile,
				size,
				"done",
				nil,
			)
		case <-time.After(30 * time.Second):
			// Timeout - kill the process
			_ = f.Close()
			if cmd.Process != nil {
				// Kill the process group to ensure it's terminated
				_ = cmd.Process.Kill()
				// Wait for it to actually die
				_ = cmd.Wait()
			}
			// Remove the incomplete file
			_ = os.Remove(outFile)
			errMsg := fmt.Errorf("Backup timed out after 30 seconds. The database may be locked by another process or very large. Try closing applications that might be using this database: %s", dbPath)
			onProgress(engine.Engine, dbIdentifier, "", 0, "failed", errMsg)
			return errMsg
		}
	}

	return nil
}

func runSQLiteAllDatabases(
	engine plan.EnginePlan,
	onProgress ProgressFunc,
) error {
	baseDir := engine.OutputDir
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

	f, err := createWritableFile(outFile)
	if err != nil {
		onProgress(engine.Engine, progressName, "", 0, "failed", err)
		return err
	}
	defer f.Close()

	for _, db := range engine.Databases {
		dbPath := db.Name
		fileInfo, err := os.Stat(dbPath)
		if os.IsNotExist(err) {
			err := fmt.Errorf("database file does not exist: %s", dbPath)
			onProgress(engine.Engine, progressName, "", 0, "failed", err)
			return err
		}
		if err != nil {
			err := fmt.Errorf("cannot access database file: %s: %v", dbPath, err)
			onProgress(engine.Engine, progressName, "", 0, "failed", err)
			return err
		}
		if fileInfo.Mode().Perm()&0444 == 0 {
			err := fmt.Errorf("database file is not readable: %s", dbPath)
			onProgress(engine.Engine, progressName, "", 0, "failed", err)
			return err
		}

		dbBaseName := strings.TrimSuffix(filepath.Base(dbPath), filepath.Ext(dbPath))
		_, _ = f.WriteString(fmt.Sprintf("\n-- SQLite dump for database: %s\n", dbBaseName))
		_, _ = f.WriteString(fmt.Sprintf("-- Source: %s\n\n", dbPath))

		var stderr bytes.Buffer
		cmd := exec.Command("sqlite3", "-batch", "-readonly", dbPath, ".dump")
		cmd.Stdout = f
		cmd.Stderr = io.MultiWriter(&stderr, os.Stderr)

		if err := cmd.Run(); err != nil {
			stderrStr := stderr.String()
			errMsg := fmt.Errorf("sqlite3 dump failed for %s: %v", dbPath, err)
			if stderrStr != "" {
				errMsg = fmt.Errorf("sqlite3 dump failed for %s: %v\n%s", dbPath, err, stderrStr)
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
