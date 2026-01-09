package plan

import "time"

type RestorePlan struct {
	CreatedAt    time.Time
	Engine       string
	Version      string
	Database     string
	RequiresAuth bool
	DumpPath     string
	RestoreDir   string // Directory for pre-restore backups
}
