package plan

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mirrorvault/pkg/model"
)

const RestoreBackupDir = "/var/backups/mirrorvault/restore-backups"

func normalizeEngineDir(engine string) string {
	return strings.ToLower(engine)
}

func Build(
	engine string,
	database string,
	dumpPath string,
	scanResult model.ScanResult,
) (*RestorePlan, error) {
	if engine == "" || database == "" {
		return nil, errors.New("engine and database must be specified")
	}

	if dumpPath == "" {
		return nil, errors.New("dump path must be specified")
	}

	// Find the database in scan result
	var db *model.Database
	for _, d := range scanResult.Databases {
		if d.Engine == engine {
			db = &d
			break
		}
	}

	if db == nil {
		return nil, errors.New("engine not found in system")
	}

	// Verify database exists
	found := false
	for _, name := range db.Names {
		if name == database {
			found = true
			break
		}
	}

	if !found {
		return nil, errors.New("database not found for the selected engine")
	}

	plan := &RestorePlan{
		CreatedAt:    time.Now(),
		Engine:       engine,
		Version:      db.Version,
		Database:     database,
		RequiresAuth: db.RequiresAuth,
		DumpPath:     dumpPath,
		RestoreDir: filepath.Join(
			RestoreBackupDir,
			normalizeEngineDir(engine),
		),
	}

	// Ensure restore backup directory exists
	if err := os.MkdirAll(plan.RestoreDir, 0755); err != nil {
		return nil, err
	}

	return plan, nil
}
