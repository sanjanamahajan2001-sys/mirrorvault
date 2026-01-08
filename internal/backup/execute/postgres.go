package execute

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
				err := fmt.Errorf("missing PostgreSQL credentials")
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

		cmd.Stdout = f
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			_ = f.Close()
			onProgress(engine.Engine, db.Name, "", 0, "failed", err)
			return err
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
