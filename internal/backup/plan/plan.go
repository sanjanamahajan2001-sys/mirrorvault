package plan

import "time"

const AllDatabasesName = "__all__"

type DatabasePlan struct {
	Name string
}

type EnginePlan struct {
	Engine       string
	Version      string
	RequiresAuth bool
	Databases    []DatabasePlan
	OutputDir    string
	AllDatabases bool
}

type BackupPlan struct {
	CreatedAt time.Time
	Engines   []EnginePlan
}
