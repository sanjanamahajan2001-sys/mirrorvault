package detect

import (
	"bytes"
	"os/exec"
	"strings"

	"mirrorvault/pkg/model"
)

func DetectMSSQL() *model.Database {
	if !CommandExists("sqlcmd") {
		return nil
	}

	verOut, _ := execOutput("sqlcmd", "-?")
	version := strings.Split(verOut, "\n")[0]

	cmd := exec.Command(
		"sudo",
		"sqlcmd",
		"-S", "localhost",
		"-Q", "SELECT name FROM sys.databases;",
	)

	var out bytes.Buffer
	cmd.Stdout = &out
	_ = cmd.Run()

	var dbs []string
	for _, line := range strings.Split(out.String(), "\n") {
		db := strings.TrimSpace(line)
		if db != "" {
			dbs = append(dbs, db)
		}
	}

	return &model.Database{
		Engine:       "MSSQL",
		Version:      version,
		Type:         model.SQL,
		RequiresAuth: true,
		Names:        dbs,
	}
}
