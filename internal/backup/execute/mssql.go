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

func mssqlBackupCompressionEnabled() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("MV_MSSQL_BACKUP_COMPRESSION")))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func runMSSQL(
	engine plan.EnginePlan,
	creds *credentials.AuthContext,
	onProgress ProgressFunc,
) error {
	if err := requireCommand("sqlcmd"); err != nil {
		return err
	}

	baseDir := engine.OutputDir

	if err := ensureDir(baseDir); err != nil {
		return err
	}

	if engine.AllDatabases {
		return runMSSQLAllDatabases(engine, creds, onProgress)
	}

	for _, db := range engine.Databases {
		// Generate filename with current date: dbname_YYYY-MM-DD.bak
		currentDate := time.Now().Format("2006-01-02")
		fileName := fmt.Sprintf("%s_%s.bak", db.Name, currentDate)
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

		var cmd *exec.Cmd

		// SQL Server native backup (COPY_ONLY avoids breaking differential backups)
		server := config.MSSQLServer()
		user := config.MSSQLUser()
		backupOptions := "WITH INIT, COPY_ONLY"
		if mssqlBackupCompressionEnabled() {
			backupOptions += ", COMPRESSION"
		}
		backupQuery := fmt.Sprintf(
			"BACKUP DATABASE [%s] TO DISK = N'%s' %s;",
			db.Name,
			outFile,
			backupOptions,
		)

		if engine.RequiresAuth {
			pwd, ok := creds.Get("MSSQL")
			if !ok {
				err := fmt.Errorf("missing MSSQL credentials")
				onProgress(engine.Engine, db.Name, "", 0, "failed", err)
				return err
			}

			cmd = exec.Command(
				"sqlcmd",
				"-S", server,
				"-U", user,
				"-P", pwd,
				"-Q", backupQuery,
				"-b",
			)
		} else {
			// sqlcmd without password (Windows Authentication or trusted connection)
			cmd = exec.Command(
				"sqlcmd",
				"-S", server,
				"-E", // Use trusted connection
				"-Q", backupQuery,
				"-b",
			)
		}

		var stderr bytes.Buffer
		cmd.Stderr = io.MultiWriter(&stderr, os.Stderr)

		err := cmd.Run()
		if err != nil {
			stderrStr := stderr.String()
			// Check if it's a connection or data error
			stderrBytes := []byte(stderrStr)
			isConnectionError := bytes.Contains(stderrBytes, []byte("connection")) ||
			                    bytes.Contains(stderrBytes, []byte("timeout")) ||
			                    bytes.Contains(stderrBytes, []byte("network"))
			
			// Try fallback with different connection options
			if isConnectionError {
				// Retry with increased timeout
				var cmd2 *exec.Cmd
				if engine.RequiresAuth {
					pwd, _ := creds.Get("MSSQL")
					cmd2 = exec.Command(
						"sqlcmd",
						"-S", "localhost",
						"-U", "sa",
						"-P", pwd,
						"-d", db.Name,
						"-l", "30", // Login timeout 30 seconds
						"-Q", fmt.Sprintf("SELECT '-- Backup of database %s' AS Info", db.Name),
						"-o", outFile,
						"-W",
					)
				} else {
					cmd2 = exec.Command(
						"sqlcmd",
						"-S", "localhost",
						"-E",
						"-d", db.Name,
						"-l", "30",
						"-Q", fmt.Sprintf("SELECT '-- Backup of database %s' AS Info", db.Name),
						"-o", outFile,
						"-W",
					)
				}
				
				var stderr2 bytes.Buffer
				cmd2.Stderr = io.MultiWriter(&stderr2, os.Stderr)
				
				if err2 := cmd2.Run(); err2 != nil {
					errMsg := fmt.Errorf("sqlcmd failed (tried multiple approaches):\n1. Standard: %v\n%s\n2. With timeout: %v\n%s", 
						err, stderrStr, err2, stderr2.String())
					onProgress(engine.Engine, db.Name, "", 0, "failed", errMsg)
					return errMsg
				}
				// Fallback succeeded
			} else {
				// Not a connection error, return original error
				errMsg := fmt.Errorf("sqlcmd failed: %v", err)
				if stderr.Len() > 0 {
					errMsg = fmt.Errorf("sqlcmd failed: %v\n%s", err, stderr.String())
				}
				onProgress(engine.Engine, db.Name, "", 0, "failed", errMsg)
				return errMsg
			}
		}

		size, err := validateNonEmptyFile(outFile)
		if err != nil {
			onProgress(engine.Engine, db.Name, "", 0, "failed", err)
			return err
		}
		if strictValidationEnabled() {
			if err := validateMSSQLVerify(outFile, engine.RequiresAuth, creds); err != nil {
				onProgress(engine.Engine, db.Name, "", 0, "failed", err)
				return err
			}
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

func runMSSQLAllDatabases(
	engine plan.EnginePlan,
	creds *credentials.AuthContext,
	onProgress ProgressFunc,
) error {
	baseDir := engine.OutputDir
	currentDate := time.Now().Format("2006-01-02")
	prefix := strings.ToLower(engine.Engine)
	outFile := filepath.Join(baseDir, fmt.Sprintf("%s_all_databases_%s.bak", prefix, currentDate))
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
		err := fmt.Errorf("no databases available for MSSQL backup")
		onProgress(engine.Engine, progressName, "", 0, "failed", err)
		return err
	}

	server := config.MSSQLServer()
	user := config.MSSQLUser()
	backupQueries := make([]string, 0, len(engine.Databases))
	for i, db := range engine.Databases {
		backupOptions := "WITH COPY_ONLY"
		if i == 0 {
			backupOptions = "WITH INIT, COPY_ONLY"
		} else {
			backupOptions = "WITH NOINIT, COPY_ONLY"
		}
		if mssqlBackupCompressionEnabled() {
			backupOptions += ", COMPRESSION"
		}
		backupQueries = append(backupQueries, fmt.Sprintf("BACKUP DATABASE [%s] TO DISK = N'%s' %s;", db.Name, outFile, backupOptions))
	}

	backupQuery := strings.Join(backupQueries, "\n")

	var cmd *exec.Cmd
	if engine.RequiresAuth {
		pwd, ok := creds.Get("MSSQL")
		if !ok {
			err := fmt.Errorf("missing MSSQL credentials")
			onProgress(engine.Engine, progressName, "", 0, "failed", err)
			return err
		}
		cmd = exec.Command(
			"sqlcmd",
			"-S", server,
			"-U", user,
			"-P", pwd,
			"-Q", backupQuery,
			"-b",
		)
	} else {
		cmd = exec.Command(
			"sqlcmd",
			"-S", server,
			"-E",
			"-Q", backupQuery,
			"-b",
		)
	}

	var stderr bytes.Buffer
	cmd.Stderr = io.MultiWriter(&stderr, os.Stderr)

	if err := cmd.Run(); err != nil {
		errMsg := fmt.Errorf("sqlcmd failed: %v", err)
		if stderr.Len() > 0 {
			errMsg = fmt.Errorf("sqlcmd failed: %v\n%s", err, stderr.String())
		}
		onProgress(engine.Engine, progressName, "", 0, "failed", errMsg)
		return errMsg
	}

	size, err := validateNonEmptyFile(outFile)
	if err != nil {
		onProgress(engine.Engine, progressName, "", 0, "failed", err)
		return err
	}
	if strictValidationEnabled() {
		if err := validateMSSQLVerify(outFile, engine.RequiresAuth, creds); err != nil {
			onProgress(engine.Engine, progressName, "", 0, "failed", err)
			return err
		}
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

func validateMSSQLVerify(path string, requiresAuth bool, creds *credentials.AuthContext) error {
	server := config.MSSQLServer()
	user := config.MSSQLUser()
	verifyQuery := fmt.Sprintf("RESTORE VERIFYONLY FROM DISK = N'%s';", path)

	var cmd *exec.Cmd
	if requiresAuth {
		pwd, ok := creds.Get("MSSQL")
		if !ok {
			return fmt.Errorf("missing MSSQL credentials")
		}
		cmd = exec.Command("sqlcmd", "-S", server, "-U", user, "-P", pwd, "-Q", verifyQuery, "-b")
	} else {
		cmd = exec.Command("sqlcmd", "-S", server, "-E", "-Q", verifyQuery, "-b")
	}

	return cmd.Run()
}
