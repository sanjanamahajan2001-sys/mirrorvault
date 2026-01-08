package plan

import "time"

type DatabasePlan struct {
	Name string
}

type EnginePlan struct {
	Engine       string
	Version      string
	RequiresAuth bool
	Databases    []DatabasePlan
	OutputDir    string
}

type BackupPlan struct {
	CreatedAt time.Time
	Engines   []EnginePlan
}
