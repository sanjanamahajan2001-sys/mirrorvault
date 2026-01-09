package execute

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"mirrorvault/internal/backup/credentials"
	"mirrorvault/internal/restore/log"
	restoreplan "mirrorvault/internal/restore/plan"
	"mirrorvault/internal/restore/validate"
)

func restoreRedis(
	restorePlan *restoreplan.RestorePlan,
	dumpPath string,
	dumpInfo *validate.DumpInfo,
	authCtx *credentials.AuthContext,
	logger *log.Logger,
	onProgress func(string, float64, string, error),
) error {
	logger.Info("Starting Redis restore")

	password := ""
	if restorePlan.RequiresAuth {
		pwd, ok := authCtx.Get("Redis")
		if !ok {
			return fmt.Errorf("missing Redis credentials")
		}
		password = pwd
	}

	// Step 1: Stop Redis (or at least flush and save)
	onProgress("Preparing Redis", 0.5, "Stopping Redis...", nil)
	logger.Info("Stopping Redis")

	var stopCmd *exec.Cmd
	if restorePlan.RequiresAuth {
		stopCmd = exec.Command("redis-cli", "-a", password, "SHUTDOWN", "SAVE")
	} else {
		stopCmd = exec.Command("redis-cli", "SHUTDOWN", "SAVE")
	}
	stopCmd.Run() // Ignore errors

	// Step 2: Find Redis data directory
	onProgress("Preparing Redis", 0.55, "Locating Redis data directory...", nil)
	logger.Info("Locating Redis data directory")

	var configCmd *exec.Cmd
	if restorePlan.RequiresAuth {
		configCmd = exec.Command("redis-cli", "-a", password, "CONFIG", "GET", "dir")
	} else {
		configCmd = exec.Command("redis-cli", "CONFIG", "GET", "dir")
	}

	configOut, err := configCmd.Output()
	var redisDir string
	if err == nil {
		// Parse output
		lines := strings.Split(strings.TrimSpace(string(configOut)), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && line != "dir" && strings.HasPrefix(line, "/") {
				redisDir = line
				break
			}
		}
	}

	// Default locations if not found
	if redisDir == "" {
		possiblePaths := []string{
			"/var/lib/redis",
			"/var/lib/redis/6379",
			"/data",
			"/tmp",
		}

		for _, path := range possiblePaths {
			if _, err := os.Stat(path); err == nil {
				redisDir = path
				break
			}
		}
	}

	if redisDir == "" {
		return fmt.Errorf("could not locate Redis data directory")
	}

	dumpFile := filepath.Join(redisDir, "dump.rdb")

	// Step 3: Copy dump to Redis data directory
	onProgress("Restoring data", 0.6, "Copying dump file...", nil)
	logger.Info("Copying dump file to Redis data directory")

	source, err := os.Open(dumpPath)
	if err != nil {
		return fmt.Errorf("failed to open dump file: %w", err)
	}
	defer source.Close()

	dest, err := os.Create(dumpFile)
	if err != nil {
		return fmt.Errorf("failed to create dump.rdb: %w", err)
	}
	defer dest.Close()

	_, err = io.Copy(dest, source)
	if err != nil {
		return fmt.Errorf("failed to copy dump file: %w", err)
	}

	// Step 4: Start Redis
	onProgress("Starting Redis", 0.9, "Starting Redis server...", nil)
	logger.Info("Starting Redis server")

	// Redis should start automatically via systemd, but we can try to start it
	startCmd := exec.Command("systemctl", "start", "redis")
	startCmd.Run() // Ignore errors - Redis might already be running

	logger.Info("Redis restore completed successfully")
	return nil
}

func rollbackRedis(
	restorePlan *restoreplan.RestorePlan,
	backupPath string,
	authCtx *credentials.AuthContext,
	logger *log.Logger,
) error {
	logger.Info("Starting Redis rollback")

	password := ""
	if restorePlan.RequiresAuth {
		pwd, ok := authCtx.Get("Redis")
		if !ok {
			return fmt.Errorf("missing Redis credentials")
		}
		password = pwd
	}

	// Stop Redis
	var stopCmd *exec.Cmd
	if restorePlan.RequiresAuth {
		stopCmd = exec.Command("redis-cli", "-a", password, "SHUTDOWN", "SAVE")
	} else {
		stopCmd = exec.Command("redis-cli", "SHUTDOWN", "SAVE")
	}
	stopCmd.Run()

	// Find Redis data directory
	var configCmd *exec.Cmd
	if restorePlan.RequiresAuth {
		configCmd = exec.Command("redis-cli", "-a", password, "CONFIG", "GET", "dir")
	} else {
		configCmd = exec.Command("redis-cli", "CONFIG", "GET", "dir")
	}

	configOut, err := configCmd.Output()
	var redisDir string
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(configOut)), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && line != "dir" && strings.HasPrefix(line, "/") {
				redisDir = line
				break
			}
		}
	}

	if redisDir == "" {
		possiblePaths := []string{
			"/var/lib/redis",
			"/var/lib/redis/6379",
			"/data",
			"/tmp",
		}

		for _, path := range possiblePaths {
			if _, err := os.Stat(path); err == nil {
				redisDir = path
				break
			}
		}
	}

	if redisDir == "" {
		return fmt.Errorf("could not locate Redis data directory")
	}

	dumpFile := filepath.Join(redisDir, "dump.rdb")

	// Restore from backup
	source, err := os.Open(backupPath)
	if err != nil {
		return fmt.Errorf("failed to open backup file: %w", err)
	}
	defer source.Close()

	dest, err := os.Create(dumpFile)
	if err != nil {
		return fmt.Errorf("failed to create dump.rdb: %w", err)
	}
	defer dest.Close()

	_, err = io.Copy(dest, source)
	if err != nil {
		return fmt.Errorf("failed to copy backup file: %w", err)
	}

	// Start Redis
	startCmd := exec.Command("systemctl", "start", "redis")
	startCmd.Run()

	logger.Info("Redis rollback completed successfully")
	return nil
}
