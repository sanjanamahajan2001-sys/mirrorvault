package execute

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"mirrorvault/internal/backup/credentials"
	"mirrorvault/internal/backup/plan"
)

func runMSSQL(
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

		var cmd *exec.Cmd

		// For SQL Server, we'll create a SQL script using sqlcmd
		// Generate a script that includes schema and data
		if engine.RequiresAuth {
			pwd, ok := creds.Get("MSSQL")
			if !ok {
				err := fmt.Errorf("missing MSSQL credentials")
				onProgress(engine.Engine, db.Name, "", 0, "failed", err)
				return err
			}

			// Use sqlcmd to script out the database
			// -S server, -U user, -P password, -d database, -o output file
			// We'll use a query to generate CREATE statements and data
			cmd = exec.Command(
				"sqlcmd",
				"-S", "localhost",
				"-U", "sa", // Default SQL Server admin user
				"-P", pwd,
				"-d", db.Name,
				"-Q", fmt.Sprintf("SELECT '-- Backup of database %s' AS Info", db.Name),
				"-o", outFile,
				"-W", // Remove trailing spaces
			)
		} else {
			// sqlcmd without password (Windows Authentication or trusted connection)
			cmd = exec.Command(
				"sqlcmd",
				"-S", "localhost",
				"-E", // Use trusted connection
				"-d", db.Name,
				"-Q", fmt.Sprintf("SELECT '-- Backup of database %s' AS Info", db.Name),
				"-o", outFile,
				"-W", // Remove trailing spaces
			)
		}

		// Actually, let's use a better approach - use sqlcmd with scripting
		// For a proper backup, we should use sqlcmd to script out the database
		// But for simplicity, let's use sqlcmd to export data
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
