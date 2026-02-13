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

func redisBackupMode() string {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("MV_REDIS_BACKUP_MODE")))
	if value == "aof" {
		return "aof"
	}
	return "rdb"
}

func runRedis(
	engine plan.EnginePlan,
	creds *credentials.AuthContext,
	onProgress ProgressFunc,
) error {
	if err := requireCommand("redis-cli"); err != nil {
		return err
	}

	baseDir := engine.OutputDir

	if err := ensureDir(baseDir); err != nil {
		return err
	}

	backupMode := redisBackupMode()

	if engine.AllDatabases {
		return runRedisAllDatabases(engine, creds, onProgress)
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
		ext := ".rdb"
		if backupMode == "aof" {
			ext = ".aof"
		}
		fileName := fmt.Sprintf("%s_%s%s", dbName, currentDate, ext)
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

		if backupMode == "rdb" {
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
		}

		var stderr bytes.Buffer
		if backupMode == "rdb" {
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
		}

		// Find and copy the dump.rdb file
		// First, try to get the directory from Redis config
		var dumpFile string
		redisDir, err := getRedisConfigValue(engine.RequiresAuth, creds, "dir")
		if err == nil && redisDir != "" && strings.HasPrefix(redisDir, "/") {
			if backupMode == "aof" {
				appendOnly := strings.ToLower(strings.TrimSpace(getRedisConfigValueOrEmpty(engine.RequiresAuth, creds, "appendonly")))
				if appendOnly != "yes" {
					err := fmt.Errorf("redis appendonly mode is disabled; cannot back up AOF")
					onProgress(engine.Engine, db.Name, "", 0, "failed", err)
					return err
				}
				appendFile := getRedisConfigValueOrEmpty(engine.RequiresAuth, creds, "appendfilename")
				if appendFile == "" {
					appendFile = "appendonly.aof"
				}
				dumpFile = filepath.Join(redisDir, appendFile)
			} else {
				dumpFile = filepath.Join(redisDir, "dump.rdb")
			}
		}

		// If not found from config, try common locations
		if dumpFile == "" {
			possiblePaths := []string{}
			if backupMode == "aof" {
				possiblePaths = []string{
					"/var/lib/redis/appendonly.aof",
					"/var/lib/redis/6379/appendonly.aof",
					"/data/appendonly.aof",
					"/tmp/appendonly.aof",
				}
			} else {
				possiblePaths = []string{
					"/var/lib/redis/dump.rdb",
					"/var/lib/redis/6379/dump.rdb",
					"/data/dump.rdb",
					"/tmp/dump.rdb",
				}
			}

			for _, path := range possiblePaths {
				if _, err := os.Stat(path); err == nil {
					dumpFile = path
					break
				}
			}
		}

		if dumpFile == "" {
			err := fmt.Errorf("could not locate Redis backup file")
			onProgress(engine.Engine, db.Name, "", 0, "failed", err)
			return err
		}

		// Copy dump to backup location
		source, err := os.Open(dumpFile)
		if err != nil {
			onProgress(engine.Engine, db.Name, "", 0, "failed", err)
			return err
		}
		defer source.Close()

		dest, err := createWritableFile(outFile)
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

		size, err := validateNonEmptyFile(outFile)
		if err != nil {
			onProgress(engine.Engine, db.Name, "", 0, "failed", err)
			return err
		}
		if backupMode == "rdb" {
			if err := validateRDB(outFile); err != nil {
				onProgress(engine.Engine, db.Name, "", 0, "failed", err)
				return err
			}
			if strictValidationEnabled() {
				if err := validateRedisCheckRDB(outFile); err != nil {
					onProgress(engine.Engine, db.Name, "", 0, "failed", err)
					return err
				}
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

func runRedisAllDatabases(
	engine plan.EnginePlan,
	creds *credentials.AuthContext,
	onProgress ProgressFunc,
) error {
	baseDir := engine.OutputDir
	backupMode := redisBackupMode()

	currentDate := time.Now().Format("2006-01-02")
	dbName := strings.ToLower(engine.Engine)
	ext := ".rdb"
	if backupMode == "aof" {
		ext = ".aof"
	}
	fileName := fmt.Sprintf("%s_%s%s", dbName, currentDate, ext)
	outFile := filepath.Join(baseDir, fileName)
	progressName := "All databases"

	onProgress(
		engine.Engine,
		progressName,
		outFile,
		0,
		"running",
		nil,
	)

	var cmd *exec.Cmd

	if backupMode == "rdb" {
		if engine.RequiresAuth {
			pwd, ok := creds.Get("Redis")
			if !ok {
				err := fmt.Errorf("missing Redis credentials")
				onProgress(engine.Engine, progressName, "", 0, "failed", err)
				return err
			}
			cmd = exec.Command("redis-cli", "-a", pwd, "SAVE")
		} else {
			cmd = exec.Command("redis-cli", "SAVE")
		}
	}

	var stderr bytes.Buffer
	if backupMode == "rdb" {
		cmd.Stderr = io.MultiWriter(&stderr, os.Stderr)
		done := make(chan error, 1)
		go func() {
			done <- cmd.Run()
		}()
		select {
		case err := <-done:
			if err != nil {
				stderrStr := stderr.String()
				if strings.Contains(stderrStr, "timeout") || strings.Contains(stderrStr, "connection") {
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
						time.Sleep(2 * time.Second)
						var lastsaveCmd *exec.Cmd
						if engine.RequiresAuth {
							pwd, _ := creds.Get("Redis")
							lastsaveCmd = exec.Command("redis-cli", "-a", pwd, "LASTSAVE")
						} else {
							lastsaveCmd = exec.Command("redis-cli", "LASTSAVE")
						}
						lastsaveCmd.Run()
					} else {
						errMsg := fmt.Errorf("Redis SAVE failed: %v\n%s\nBGSAVE fallback also failed: %v\n%s",
							err, stderrStr, err2, bgsaveStderr.String())
						onProgress(engine.Engine, progressName, "", 0, "failed", errMsg)
						return errMsg
					}
				} else {
					errMsg := fmt.Errorf("Redis SAVE failed: %v", err)
					if stderr.Len() > 0 {
						errMsg = fmt.Errorf("Redis SAVE failed: %v\n%s", err, stderr.String())
					}
					onProgress(engine.Engine, progressName, "", 0, "failed", errMsg)
					return errMsg
				}
			}
		case <-time.After(60 * time.Second):
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			}
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
				onProgress(engine.Engine, progressName, "", 0, "failed", errMsg)
				return errMsg
			}
			time.Sleep(5 * time.Second)
		}
	}

	var dumpFile string
	redisDir, err := getRedisConfigValue(engine.RequiresAuth, creds, "dir")
	if err == nil && redisDir != "" && strings.HasPrefix(redisDir, "/") {
		if backupMode == "aof" {
			appendOnly := strings.ToLower(strings.TrimSpace(getRedisConfigValueOrEmpty(engine.RequiresAuth, creds, "appendonly")))
			if appendOnly != "yes" {
				err := fmt.Errorf("redis appendonly mode is disabled; cannot back up AOF")
				onProgress(engine.Engine, progressName, "", 0, "failed", err)
				return err
			}
			appendFile := getRedisConfigValueOrEmpty(engine.RequiresAuth, creds, "appendfilename")
			if appendFile == "" {
				appendFile = "appendonly.aof"
			}
			dumpFile = filepath.Join(redisDir, appendFile)
		} else {
			dumpFile = filepath.Join(redisDir, "dump.rdb")
		}
	}

	if dumpFile == "" {
		possiblePaths := []string{}
		if backupMode == "aof" {
			possiblePaths = []string{
				"/var/lib/redis/appendonly.aof",
				"/var/lib/redis/6379/appendonly.aof",
				"/data/appendonly.aof",
				"/tmp/appendonly.aof",
			}
		} else {
			possiblePaths = []string{
				"/var/lib/redis/dump.rdb",
				"/var/lib/redis/6379/dump.rdb",
				"/data/dump.rdb",
				"/tmp/dump.rdb",
			}
		}

		for _, path := range possiblePaths {
			if _, err := os.Stat(path); err == nil {
				dumpFile = path
				break
			}
		}
	}

	if dumpFile == "" {
		err := fmt.Errorf("could not locate Redis backup file")
		onProgress(engine.Engine, progressName, "", 0, "failed", err)
		return err
	}

	source, err := os.Open(dumpFile)
	if err != nil {
		onProgress(engine.Engine, progressName, "", 0, "failed", err)
		return err
	}
	defer source.Close()

	dest, err := createWritableFile(outFile)
	if err != nil {
		onProgress(engine.Engine, progressName, "", 0, "failed", err)
		return err
	}
	defer dest.Close()

	if _, err := io.Copy(dest, source); err != nil {
		onProgress(engine.Engine, progressName, "", 0, "failed", err)
		return err
	}

	size, err := validateNonEmptyFile(outFile)
	if err != nil {
		onProgress(engine.Engine, progressName, "", 0, "failed", err)
		return err
	}
	if backupMode == "rdb" {
		if err := validateRDB(outFile); err != nil {
			onProgress(engine.Engine, progressName, "", 0, "failed", err)
			return err
		}
		if strictValidationEnabled() {
			if err := validateRedisCheckRDB(outFile); err != nil {
				onProgress(engine.Engine, progressName, "", 0, "failed", err)
				return err
			}
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

func validateRedisCheckRDB(path string) error {
	if _, err := exec.LookPath("redis-check-rdb"); err != nil {
		return nil
	}
	cmd := exec.Command("redis-check-rdb", path)
	return cmd.Run()
}

func getRedisConfigValue(requiresAuth bool, creds *credentials.AuthContext, key string) (string, error) {
	var cmd *exec.Cmd
	if requiresAuth {
		pwd, _ := creds.Get("Redis")
		cmd = exec.Command("redis-cli", "-a", pwd, "CONFIG", "GET", key)
	} else {
		cmd = exec.Command("redis-cli", "CONFIG", "GET", key)
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.EqualFold(line, key) {
			continue
		}
		return line, nil
	}
	return "", nil
}

func getRedisConfigValueOrEmpty(requiresAuth bool, creds *credentials.AuthContext, key string) string {
	value, err := getRedisConfigValue(requiresAuth, creds, key)
	if err != nil {
		return ""
	}
	return value
}
