package plan

import (
	"errors"
	"path/filepath"
	"strings"
	"time"

	"mirrorvault/pkg/model"
)

const DefaultBackupDir = "/var/backups/mirrorvault"

func normalizeEngineDir(engine string) string {
	return strings.ToLower(engine)
}

func Build(
	scan model.ScanResult,
	selected map[string][]string,
) (*BackupPlan, error) {

	if len(selected) == 0 {
		return nil, errors.New("no databases selected")
	}

	plan := &BackupPlan{
		CreatedAt: time.Now(),
	}

	for _, db := range scan.Databases {
		dbNames, ok := selected[db.Engine]
		if !ok || len(dbNames) == 0 {
			continue
		}

		enginePlan := EnginePlan{
			Engine:       db.Engine,
			Version:      db.Version,
			RequiresAuth: db.RequiresAuth,
			OutputDir: filepath.Join(
				DefaultBackupDir,
				normalizeEngineDir(db.Engine),
			),
		}

		for _, name := range dbNames {
			enginePlan.Databases = append(enginePlan.Databases, DatabasePlan{
				Name: name,
			})
		}

		plan.Engines = append(plan.Engines, enginePlan)
	}

	if len(plan.Engines) == 0 {
		return nil, errors.New("nothing to back up after planning")
	}

	return plan, nil
}
