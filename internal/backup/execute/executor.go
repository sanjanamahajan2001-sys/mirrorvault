package execute

import (
	"context"
	"fmt"
	"os"

	"mirrorvault/internal/backup/credentials"
	"mirrorvault/internal/backup/plan"
	"mirrorvault/internal/drive"
)

type ProgressFunc func(engine, db, path string, size int64, status string, err error)
type DriveProgressFunc func(engine, db string, progress DriveProgress)

type DriveProgress struct {
	Stage            string
	Message          string
	RemoteName       string
	BackupSize       int64
	AccountRemaining int64
	AccountTotal     int64
	FolderSize       int64
	Err              error
}

func Run(
	p *plan.BackupPlan,
	creds *credentials.AuthContext,
	onProgress ProgressFunc,
	onDriveProgress DriveProgressFunc,
	enableDrive bool,
) error {
	// Safety check to prevent nil pointer dereference
	if p == nil {
		return fmt.Errorf("backup plan is nil")
	}
	if creds == nil {
		// Create empty context if nil (for databases that don't require auth)
		creds = credentials.NewContext()
	}
	if onProgress == nil {
		return fmt.Errorf("progress callback is nil")
	}

	var driveClient *drive.Client
	var driveCfg *drive.Config
	var driveQuota *drive.AccountQuota
	driveSkipReason := ""
	driveEnabled := enableDrive && os.Getenv("MIRRORVAULT_SCHEDULED") != "true"
	if !enableDrive {
		driveSkipReason = "Drive disabled in settings"
	} else if os.Getenv("MIRRORVAULT_SCHEDULED") == "true" {
		driveSkipReason = "Drive uploads are disabled for scheduled backups"
	}
	if driveEnabled {
		cfg, err := drive.LoadConfig()
		if err != nil {
			driveEnabled = false
			if driveSkipReason == "" {
				driveSkipReason = "Drive config not available"
			}
			if onDriveProgress != nil {
				onDriveProgress("", "", DriveProgress{
					Stage:   "skipped",
					Message: "Drive config not available",
					Err:     err,
				})
			}
		} else if cfg.Enabled && cfg.IsConfigured() {
			client, err := drive.NewClient(context.Background(), cfg)
			if err != nil {
				driveEnabled = false
				if driveSkipReason == "" {
					driveSkipReason = "Drive connection failed"
				}
				if onDriveProgress != nil {
					onDriveProgress("", "", DriveProgress{
						Stage:   "skipped",
						Message: "Drive connection failed",
						Err:     err,
					})
				}
			} else {
				driveClient = client
				driveCfg = cfg
				quota, err := drive.GetAccountQuota(context.Background(), driveClient.Service)
				if err == nil {
					driveQuota = quota
				}
			}
		} else {
			driveEnabled = false
			if driveSkipReason == "" {
				if cfg != nil && !cfg.Enabled {
					driveSkipReason = "Drive disabled in config"
				} else {
					driveSkipReason = "Drive not connected"
				}
			}
		}
	}

	wrappedProgress := func(engine, db, path string, size int64, status string, err error) {
		onProgress(engine, db, path, size, status, err)

		if status == "done" && err == nil && (!driveEnabled || driveClient == nil || driveCfg == nil) {
			if onDriveProgress != nil {
				message := driveSkipReason
				if message == "" {
					message = "Drive is not configured"
				}
				onDriveProgress(engine, db, DriveProgress{
					Stage:   "skipped",
					Message: message,
				})
			}
			return
		}

		if status != "done" || err != nil || !driveEnabled || driveClient == nil || driveCfg == nil {
			return
		}

		if driveCfg.FolderID == "" {
			if onDriveProgress != nil {
				onDriveProgress(engine, db, DriveProgress{
					Stage:   "skipped",
					Message: "Drive folder not configured. Local backup stored.",
				})
			}
			return
		}

		backupSize := size
		if backupSize <= 0 && path != "" {
			if localSize, err := drive.LocalPathSize(path); err == nil {
				backupSize = localSize
			}
		}

		if onDriveProgress != nil {
			onDriveProgress(engine, db, DriveProgress{
				Stage:            "checking",
				Message:          "Checking Google Drive free space",
				BackupSize:       backupSize,
				AccountRemaining: quotaRemaining(driveQuota),
				AccountTotal:     quotaTotal(driveQuota),
			})
		}

		if driveQuota != nil && driveQuota.Limit > 0 {
			required := backupSize * 2
			if driveQuota.Remaining < required {
				if onDriveProgress != nil {
					onDriveProgress(engine, db, DriveProgress{
						Stage:            "skipped",
						Message:          fmt.Sprintf("Drive space is insufficient. Local backup stored at %s", path),
						BackupSize:       backupSize,
						AccountRemaining: driveQuota.Remaining,
						AccountTotal:     driveQuota.Limit,
					})
				}
				return
			}
		}

		uploadPath := path
		tempFile := ""
		if isDir(path) {
			archive, _, err := drive.TarGzDirectory(path)
			if err != nil {
				if onDriveProgress != nil {
					onDriveProgress(engine, db, DriveProgress{
						Stage:   "failed",
						Message: "Failed to package directory for upload",
						Err:     err,
					})
				}
				return
			}
			uploadPath = archive
			tempFile = archive
		}

		if onDriveProgress != nil {
			onDriveProgress(engine, db, DriveProgress{
				Stage:      "uploading",
				Message:    "Uploading backup to Google Drive",
				BackupSize: backupSize,
			})
		}

		result, err := drive.UploadFile(context.Background(), driveClient.Service, driveCfg.FolderID, uploadPath)
		if tempFile != "" {
			_ = os.Remove(tempFile)
		}
		if err != nil {
			if onDriveProgress != nil {
				onDriveProgress(engine, db, DriveProgress{
					Stage:   "failed",
					Message: "Drive upload failed",
					Err:     err,
				})
			}
			return
		}
		if onDriveProgress != nil {
			onDriveProgress(engine, db, DriveProgress{
				Stage:      "done",
				Message:    "Backup uploaded to Google Drive",
				RemoteName: result.Name,
				BackupSize: result.Size,
			})
		}
	}

	for _, engine := range p.Engines {
		switch engine.Engine {

		case "MySQL":
			if err := runMySQL(engine, creds, wrappedProgress); err != nil {
				return err
			}

		case "PostgreSQL":
			if err := runPostgreSQL(engine, creds, wrappedProgress); err != nil {
				return err
			}

		case "Redis":
			if err := runRedis(engine, creds, wrappedProgress); err != nil {
				return err
			}

		case "MongoDB":
			if err := runMongoDB(engine, creds, wrappedProgress); err != nil {
				return err
			}

		case "SQLite":
			if err := runSQLite(engine, creds, wrappedProgress); err != nil {
				return err
			}

		case "MSSQL":
			if err := runMSSQL(engine, creds, wrappedProgress); err != nil {
				return err
			}

		default:
			fmt.Printf("⚠ Skipping %s (execution not implemented yet)\n", engine.Engine)
		}
	}

	return nil
}

func isDir(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func quotaRemaining(quota *drive.AccountQuota) int64 {
	if quota == nil {
		return 0
	}
	return quota.Remaining
}

func quotaTotal(quota *drive.AccountQuota) int64 {
	if quota == nil {
		return 0
	}
	return quota.Limit
}
