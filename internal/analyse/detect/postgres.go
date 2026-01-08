package detect

import (
	"bytes"
	"os/exec"
	"strings"

	"mirrorvault/pkg/model"
)

func DetectPostgres() *model.Database {
	if !CommandExists("psql") {
		return nil
	}

	versionRaw, _ := execOutput("psql", "--version")

	// Auth detection
	authRequired := true
	authCheck := exec.Command("sudo", "-u", "postgres", "psql", "-c", "SELECT 1;")
	if err := authCheck.Run(); err == nil {
		authRequired = false
	}

	// Enumerate DBs
	cmd := exec.Command("sudo", "-u", "postgres", "psql", "-lqt")
	var out bytes.Buffer
	cmd.Stdout = &out

	var dbs []string
	if err := cmd.Run(); err == nil {
		for _, line := range strings.Split(out.String(), "\n") {
			parts := strings.Split(line, "|")
			if len(parts) == 0 {
				continue
			}
			db := strings.TrimSpace(parts[0])
			if db == "" || db == "postgres" || strings.HasPrefix(db, "template") {
				continue
			}
			dbs = append(dbs, db)
		}
	}

	return &model.Database{
		Engine:       "PostgreSQL",
		Version:      normalizeVersion(versionRaw),
		Type:         model.SQL,
		RequiresAuth: authRequired,
		Names:        dbs,
	}
}
