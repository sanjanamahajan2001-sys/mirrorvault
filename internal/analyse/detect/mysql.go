package detect

import (
	"bytes"
	"os/exec"
	"strings"

	"mirrorvault/pkg/model"
)

func DetectMySQL() *model.Database {
	if !CommandExists("mysql") {
		return nil
	}

	versionRaw, _ := execOutput("mysql", "--version")

	// Detect auth capability
	authRequired := true
	authCheck := exec.Command("sudo", "mysql", "-u", "root", "-e", "SELECT 1;")
	if err := authCheck.Run(); err == nil {
		authRequired = false
	}

	// Enumerate DBs
	cmd := exec.Command("sudo", "mysql", "-N", "-e", "SHOW DATABASES;")
	var out bytes.Buffer
	cmd.Stdout = &out

	var dbs []string
	if err := cmd.Run(); err == nil {
		for _, db := range strings.Split(out.String(), "\n") {
			db = strings.TrimSpace(db)
			if db == "" {
				continue
			}
			switch db {
			case "mysql", "information_schema", "performance_schema", "sys":
				continue
			}
			dbs = append(dbs, db)
		}
	}

	return &model.Database{
		Engine:       "MySQL",
		Version:      normalizeVersion(versionRaw),
		Type:         model.SQL,
		RequiresAuth: authRequired,
		Names:        dbs,
	}
}

