package detect

import (
	"bytes"
	"os/exec"
	"strings"

	"mirrorvault/pkg/model"
)

func DetectMongoDB() *model.Database {
	if !CommandExists("mongod") && !CommandExists("mongosh") {
		return nil
	}

	// ---- Version detection ----
	versionRaw, err := execOutput("mongod", "--version")
	version := "unknown"
	if err == nil {
		for _, line := range strings.Split(versionRaw, "\n") {
			if strings.Contains(line, "db version") {
				// Example: "db version v6.0.5"
				parts := strings.Fields(line)
				if len(parts) > 2 {
					version = strings.TrimPrefix(parts[len(parts)-1], "v")
				}
				break
			}
		}
	}

	// ---- Auth capability detection ----
	requiresAuth := true

	authCheck := exec.Command(
		"mongosh",
		"--quiet",
		"--eval",
		"db.runCommand({ ping: 1 })",
	)

	if err := authCheck.Run(); err == nil {
		// Able to connect without creds
		requiresAuth = false
	}

	// ---- Database enumeration ----
	cmd := exec.Command(
		"mongosh",
		"--quiet",
		"--eval",
		"db.adminCommand('listDatabases').databases.map(d => d.name).join('\\n')",
	)

	var out bytes.Buffer
	cmd.Stdout = &out
	_ = cmd.Run()

	var dbs []string
	for _, line := range strings.Split(out.String(), "\n") {
		name := strings.TrimSpace(line)
		if name != "" {
			dbs = append(dbs, name)
		}
	}

	return &model.Database{
		Engine:       "MongoDB",
		Version:      version,
		Type:         model.NoSQL,
		RequiresAuth: requiresAuth,
		Names:        dbs,
	}
}
