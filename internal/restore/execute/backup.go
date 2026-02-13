package execute

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mirrorvault/internal/backup/credentials"
	backupexecute "mirrorvault/internal/backup/execute"
	backupplan "mirrorvault/internal/backup/plan"
	"mirrorvault/internal/restore/log"
	restoreplan "mirrorvault/internal/restore/plan"
)

func createPreRestoreBackup(
	restorePlan *restoreplan.RestorePlan,
	authCtx *credentials.AuthContext,
	logger *log.Logger,
) (string, error) {
	// Create backup plan for pre-restore backup
	backupPlan := &backupplan.BackupPlan{
		Engines: []backupplan.EnginePlan{
			{
				Engine:       restorePlan.Engine,
				Version:      restorePlan.Version,
				RequiresAuth: restorePlan.RequiresAuth,
				OutputDir:    restorePlan.RestoreDir,
				Databases: []backupplan.DatabasePlan{
					{Name: restorePlan.Database},
				},
			},
		},
	}

	// Generate filename with current date and pre-restore suffix
	currentDate := time.Now().Format("2006-01-02")
	
	var backupPath string
	var backupErr error

	// Use progress callback to capture backup path
	onProgress := func(engine, db, path string, size int64, status string, err error) {
		if status == "done" && path != "" {
			backupPath = path
			logger.Info(fmt.Sprintf("Backup callback received path: %s (status: %s)", path, status))
		}
		if err != nil {
			backupErr = err
		}
	}

	// Execute backup
	err := backupexecute.Run(backupPlan, authCtx, onProgress, nil, false)
	if err != nil {
		return "", fmt.Errorf("backup execution failed: %w", err)
	}

	if backupErr != nil {
		return "", backupErr
	}

	// If backup path wasn't set, construct it manually using the same format as backup system
	if backupPath == "" {
		ext := ".sql"
		dirFormat := false
		switch restorePlan.Engine {
		case "MongoDB":
			mongoFormat := strings.TrimSpace(strings.ToLower(os.Getenv("MV_MONGO_BACKUP_FORMAT")))
			if mongoFormat == "archive" || mongoFormat == "archive.gz" || mongoFormat == "archive_gz" {
				ext = ".archive"
				if strings.Contains(mongoFormat, "gz") {
					ext = ".archive.gz"
				}
			} else {
				ext = ""
				dirFormat = true
			}
		case "Redis":
			if strings.TrimSpace(strings.ToLower(os.Getenv("MV_REDIS_BACKUP_MODE"))) == "aof" {
				ext = ".aof"
			} else {
				ext = ".rdb"
			}
		case "SQLite":
			if strings.TrimSpace(strings.ToLower(os.Getenv("MV_SQLITE_BACKUP_MODE"))) == "backup" {
				ext = ".db"
			}
		case "PostgreSQL":
			backupFormat := strings.TrimSpace(strings.ToLower(os.Getenv("MV_POSTGRES_BACKUP_FORMAT")))
			if backupFormat == "custom" || backupFormat == "c" {
				ext = ".dump"
			} else if backupFormat == "directory" || backupFormat == "dir" || backupFormat == "d" {
				ext = ""
				dirFormat = true
			}
		case "MSSQL":
			ext = ".bak"
		}

		if dirFormat || ext == "" {
			backupPath = filepath.Join(restorePlan.RestoreDir, fmt.Sprintf("%s_%s", restorePlan.Database, currentDate))
		} else {
			backupPath = filepath.Join(restorePlan.RestoreDir, fmt.Sprintf("%s_%s%s", restorePlan.Database, currentDate, ext))
		}

		backupCompression := strings.TrimSpace(strings.ToLower(os.Getenv("MV_BACKUP_COMPRESSION")))
		if backupCompression != "" && !dirFormat && ext != "" {
			switch backupCompression {
			case "gz":
				backupPath += ".gz"
			case "bz2":
				backupPath += ".bz2"
			case "zip":
				backupPath += ".zip"
			}
		} else if backupCompression != "" && dirFormat {
			switch backupCompression {
			case "gz":
				backupPath += ".tar.gz"
			case "bz2":
				backupPath += ".tar.bz2"
			case "zip":
				backupPath += ".zip"
			}
		}
		logger.Warning(fmt.Sprintf("Backup path not captured from callback, constructed: %s", backupPath))
	}

	// Verify backup file exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return "", fmt.Errorf("backup file does not exist at expected path: %s", backupPath)
	}

	logger.Info(fmt.Sprintf("Pre-restore backup created at: %s", backupPath))
	return backupPath, nil
}
