package execute

import (
	"fmt"
	"os"
	"path/filepath"
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
	err := backupexecute.Run(backupPlan, authCtx, onProgress)
	if err != nil {
		return "", fmt.Errorf("backup execution failed: %w", err)
	}

	if backupErr != nil {
		return "", backupErr
	}

	// If backup path wasn't set, construct it manually using the same format as backup system
	if backupPath == "" {
		// Backup system uses format: {dbname}_{YYYY-MM-DD}.sql
		// Determine file extension based on engine
		ext := ".sql"
		if restorePlan.Engine == "MongoDB" {
			ext = ""
		} else if restorePlan.Engine == "Redis" {
			ext = ".rdb"
		}

		if restorePlan.Engine == "MongoDB" {
			backupPath = filepath.Join(restorePlan.RestoreDir, fmt.Sprintf("%s_%s", restorePlan.Database, currentDate))
		} else {
			backupPath = filepath.Join(restorePlan.RestoreDir, fmt.Sprintf("%s_%s%s", restorePlan.Database, currentDate, ext))
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
