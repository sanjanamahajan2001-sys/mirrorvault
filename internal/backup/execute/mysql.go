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

		var cmd *exec.Cmd

		// Match the scan logic: always use sudo for consistency
		// The scan uses "sudo mysql", so we use "sudo mysqldump"
		if engine.RequiresAuth {
			pwd, ok := creds.Get("MySQL")
			if !ok {
				err := fmt.Errorf("missing MySQL credentials")
				onProgress(engine.Engine, db.Name, "", 0, "failed", err)
				return err
			}

			// Use -p format with password directly attached (no space) - standard mysqldump format
			// Format: -pPASSWORD (not -p PASSWORD)
			cmd = exec.Command(
				"sudo",
				"mysqldump",
				"-u", "root",
				"-p" + pwd,
				db.Name,
			)
		} else {
			// No password required
			cmd = exec.Command(
				"sudo",
				"mysqldump",
				"-u", "root",
				db.Name,
			)
		}

		f, err := os.Create(outFile)
		if err != nil {
			onProgress(engine.Engine, db.Name, "", 0, "failed", err)
			return err
		}

		// Capture stderr to include in error message
		var stderr bytes.Buffer
		cmd.Stdout = f
		cmd.Stderr = &stderr
		// Also write to os.Stderr so user can see it in terminal
		cmd.Stderr = io.MultiWriter(&stderr, os.Stderr)

		if err := cmd.Run(); err != nil {
			_ = f.Close()
			// Include stderr in error for better debugging
			errMsg := fmt.Errorf("mysqldump failed: %v", err)
			if stderr.Len() > 0 {
				errMsg = fmt.Errorf("mysqldump failed: %v\n%s", err, stderr.String())
			}
			onProgress(engine.Engine, db.Name, "", 0, "failed", errMsg)
			return errMsg
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
