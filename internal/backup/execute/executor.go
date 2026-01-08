package execute

import (
	"fmt"

	"mirrorvault/internal/backup/credentials"
	"mirrorvault/internal/backup/plan"
)

type ProgressFunc func(engine, db, path string, size int64, status string, err error)

func Run(
	p *plan.BackupPlan,
	creds *credentials.AuthContext,
	onProgress ProgressFunc,
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

	for _, engine := range p.Engines {
		switch engine.Engine {

		case "MySQL":
			if err := runMySQL(engine, creds, onProgress); err != nil {
				return err
			}

		case "PostgreSQL":
			if err := runPostgreSQL(engine, creds, onProgress); err != nil {
				return err
			}

		case "Redis":
			if err := runRedis(engine, creds, onProgress); err != nil {
				return err
			}

		case "MongoDB":
			if err := runMongoDB(engine, creds, onProgress); err != nil {
				return err
			}

		case "SQLite":
			if err := runSQLite(engine, creds, onProgress); err != nil {
				return err
			}

		case "MSSQL":
			if err := runMSSQL(engine, creds, onProgress); err != nil {
				return err
			}

		default:
			fmt.Printf("⚠ Skipping %s (execution not implemented yet)\n", engine.Engine)
		}
	}

	return nil
}
