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

func runMongoDB(
	engine plan.EnginePlan,
	creds *credentials.AuthContext,
	onProgress ProgressFunc,
) error {

	baseDir := engine.OutputDir

	if err := ensureDir(baseDir); err != nil {
		return err
	}

	for _, db := range engine.Databases {
		// Generate filename with current date: dbname_YYYY-MM-DD
		currentDate := time.Now().Format("2006-01-02")
		// MongoDB backup creates a directory, so we'll name it dbname_YYYY-MM-DD
		backupDirName := fmt.Sprintf("%s_%s", db.Name, currentDate)
		outDir := filepath.Join(baseDir, backupDirName)

		// ▶ running
		onProgress(
			engine.Engine,
			db.Name,
			outDir,
			0,
			"running",
			nil,
		)

		var cmd *exec.Cmd

		if engine.RequiresAuth {
			pwd, ok := creds.Get("MongoDB")
			if !ok {
				err := fmt.Errorf("missing MongoDB credentials")
				onProgress(engine.Engine, db.Name, "", 0, "failed", err)
				return err
			}

			// mongodump with authentication
			// Use "admin" as default username (most common MongoDB setup)
			// Note: In production, username might need to be configurable
			cmd = exec.Command(
				"mongodump",
				"--db", db.Name,
				"--out", outDir,
				"--username", "admin",
				"--password", pwd,
				"--authenticationDatabase", "admin",
			)
		} else {
			// mongodump without authentication
			cmd = exec.Command(
				"mongodump",
				"--db", db.Name,
				"--out", outDir,
			)
		}

		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			onProgress(engine.Engine, db.Name, "", 0, "failed", err)
			return err
		}

		// Calculate total size of backup directory
		var size int64
		err := filepath.Walk(outDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				size += info.Size()
			}
			return nil
		})

		if err != nil {
			// If size calculation fails, just report 0
			size = 0
		}

		// ✔ done
		onProgress(
			engine.Engine,
			db.Name,
			outDir,
			size,
			"done",
			nil,
		)
	}

	return nil
}
