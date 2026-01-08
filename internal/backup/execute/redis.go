package execute

import (
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

func runRedis(
	engine plan.EnginePlan,
	creds *credentials.AuthContext,
	onProgress ProgressFunc,
) error {

	baseDir := engine.OutputDir

	if err := ensureDir(baseDir); err != nil {
		return err
	}

	// Redis typically has one dump file, but we'll handle each "database" entry
	for _, db := range engine.Databases {
		// Generate filename with current date: dbname_YYYY-MM-DD.rdb
		currentDate := time.Now().Format("2006-01-02")
		// For Redis, db.Name is typically "dump.rdb", extract base name
		dbName := "redis"
		if db.Name != "dump.rdb" {
			dbName = db.Name
		}
		fileName := fmt.Sprintf("%s_%s.rdb", dbName, currentDate)
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

		// Redis backup: Use SAVE command to create dump.rdb synchronously
		if engine.RequiresAuth {
			pwd, ok := creds.Get("Redis")
			if !ok {
				err := fmt.Errorf("missing Redis credentials")
				onProgress(engine.Engine, db.Name, "", 0, "failed", err)
				return err
			}

			cmd = exec.Command(
				"redis-cli",
				"-a", pwd,
				"SAVE",
			)
		} else {
			cmd = exec.Command(
				"redis-cli",
				"SAVE",
			)
		}

		cmd.Stderr = os.Stderr

		// Run SAVE command
		if err := cmd.Run(); err != nil {
			onProgress(engine.Engine, db.Name, "", 0, "failed", err)
			return err
		}

		// Find and copy the dump.rdb file
		// First, try to get the directory from Redis config
		var dumpFile string
		var configCmd *exec.Cmd
		if engine.RequiresAuth {
			pwd, _ := creds.Get("Redis")
			configCmd = exec.Command("redis-cli", "-a", pwd, "CONFIG", "GET", "dir")
		} else {
			configCmd = exec.Command("redis-cli", "CONFIG", "GET", "dir")
		}

		configOut, err := configCmd.Output()
		if err == nil {
			// Parse output: format is "dir\n/path/to/dir\n"
			lines := strings.Split(strings.TrimSpace(string(configOut)), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" && line != "dir" && strings.HasPrefix(line, "/") {
					dumpFile = filepath.Join(line, "dump.rdb")
					break
				}
			}
		}

		// If not found from config, try common locations
		if dumpFile == "" {
			possiblePaths := []string{
				"/var/lib/redis/dump.rdb",
				"/var/lib/redis/6379/dump.rdb",
				"/data/dump.rdb",
				"/tmp/dump.rdb",
			}

			for _, path := range possiblePaths {
				if _, err := os.Stat(path); err == nil {
					dumpFile = path
					break
				}
			}
		}

		if dumpFile == "" {
			err := fmt.Errorf("could not locate Redis dump.rdb file")
			onProgress(engine.Engine, db.Name, "", 0, "failed", err)
			return err
		}

		// Copy dump.rdb to backup location
		source, err := os.Open(dumpFile)
		if err != nil {
			onProgress(engine.Engine, db.Name, "", 0, "failed", err)
			return err
		}
		defer source.Close()

		dest, err := os.Create(outFile)
		if err != nil {
			onProgress(engine.Engine, db.Name, "", 0, "failed", err)
			return err
		}
		defer dest.Close()

		_, err = io.Copy(dest, source)
		if err != nil {
			onProgress(engine.Engine, db.Name, "", 0, "failed", err)
			return err
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
