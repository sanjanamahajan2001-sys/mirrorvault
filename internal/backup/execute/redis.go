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

		var stderr bytes.Buffer
		cmd.Stderr = io.MultiWriter(&stderr, os.Stderr)

		// Run SAVE command with timeout handling
		done := make(chan error, 1)
		go func() {
			done <- cmd.Run()
		}()

		// Wait for either completion or timeout (60 seconds for large datasets)
		select {
		case err := <-done:
			if err != nil {
				stderrStr := stderr.String()
				// Try fallback with BGSAVE (non-blocking) if SAVE fails
				if strings.Contains(stderrStr, "timeout") || strings.Contains(stderrStr, "connection") {
					// Try BGSAVE as fallback
					var bgsaveCmd *exec.Cmd
					if engine.RequiresAuth {
						pwd, _ := creds.Get("Redis")
						bgsaveCmd = exec.Command("redis-cli", "-a", pwd, "BGSAVE")
					} else {
						bgsaveCmd = exec.Command("redis-cli", "BGSAVE")
					}
					
					var bgsaveStderr bytes.Buffer
					bgsaveCmd.Stderr = io.MultiWriter(&bgsaveStderr, os.Stderr)
					
					if err2 := bgsaveCmd.Run(); err2 == nil {
						// Wait a bit for BGSAVE to complete
						time.Sleep(2 * time.Second)
						// Check if save is in progress
						var lastsaveCmd *exec.Cmd
						if engine.RequiresAuth {
							pwd, _ := creds.Get("Redis")
							lastsaveCmd = exec.Command("redis-cli", "-a", pwd, "LASTSAVE")
						} else {
							lastsaveCmd = exec.Command("redis-cli", "LASTSAVE")
						}
						lastsaveCmd.Run() // Just trigger, don't check error
					} else {
						errMsg := fmt.Errorf("Redis SAVE failed: %v\n%s\nBGSAVE fallback also failed: %v\n%s", 
							err, stderrStr, err2, bgsaveStderr.String())
						onProgress(engine.Engine, db.Name, "", 0, "failed", errMsg)
						return errMsg
					}
				} else {
					errMsg := fmt.Errorf("Redis SAVE failed: %v", err)
					if stderr.Len() > 0 {
						errMsg = fmt.Errorf("Redis SAVE failed: %v\n%s", err, stderr.String())
					}
					onProgress(engine.Engine, db.Name, "", 0, "failed", errMsg)
					return errMsg
				}
			}
		case <-time.After(60 * time.Second):
			// Timeout - kill the process and try BGSAVE
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			}
			
			// Try BGSAVE as fallback
			var bgsaveCmd *exec.Cmd
			if engine.RequiresAuth {
				pwd, _ := creds.Get("Redis")
				bgsaveCmd = exec.Command("redis-cli", "-a", pwd, "BGSAVE")
			} else {
				bgsaveCmd = exec.Command("redis-cli", "BGSAVE")
			}
			
			var bgsaveStderr bytes.Buffer
			bgsaveCmd.Stderr = io.MultiWriter(&bgsaveStderr, os.Stderr)
			
			if err := bgsaveCmd.Run(); err != nil {
				errMsg := fmt.Errorf("Redis SAVE timed out after 60 seconds. BGSAVE fallback also failed: %v\n%s", err, bgsaveStderr.String())
				onProgress(engine.Engine, db.Name, "", 0, "failed", errMsg)
				return errMsg
			}
			
			// Wait for BGSAVE to complete
			time.Sleep(5 * time.Second)
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
