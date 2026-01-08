package model

import "strings"

type DatabaseType string

const (
	SQL   DatabaseType = "SQL"
	NoSQL DatabaseType = "NoSQL"
)

type Database struct {
	ID           int
	Engine       string
	Version      string
	Type         DatabaseType
	Names        []string
	RequiresAuth bool
	Running      bool
}

func (d Database) DisplayVersion() string {
	v := d.Version
	if idx := strings.Index(v, "-"); idx != -1 {
		return v[:idx]
	}
	return v
}

type ScanResult struct {
	Databases []Database
}
