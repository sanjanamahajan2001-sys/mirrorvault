package schedule

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const cronDir = "/etc/cron.d"

func cronFilePath(timerName string) string {
	baseName := strings.TrimSuffix(timerName, ".timer")
	return filepath.Join(cronDir, baseName)
}

func createCronSchedule(schedule Schedule, mirrorvaultPath string) error {
	timeParts := strings.Split(schedule.Time, ":")
	if len(timeParts) != 2 {
		return fmt.Errorf("invalid time format: %s (expected HH:MM)", schedule.Time)
	}
	hour := timeParts[0]
	minute := timeParts[1]

	dbList := strings.Join(schedule.Databases, " ")
	secretPath, hasSecret := secretFileExists(schedule.TimerName)
	command := buildCronCommand(mirrorvaultPath, schedule.Engine, dbList, schedule.Compression, secretPath, hasSecret, false)
	catchupCommand := buildCronCommand(mirrorvaultPath, schedule.Engine, dbList, schedule.Compression, secretPath, hasSecret, true)

	content := fmt.Sprintf(`SHELL=/bin/sh
PATH=/usr/local/sbin:/usr/local/bin:/sbin:/bin:/usr/sbin:/usr/bin
%s %s * * * root %s
@reboot root %s
`, minute, hour, command, catchupCommand)

	cronPath := cronFilePath(schedule.TimerName)
	if err := os.WriteFile(cronPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write cron file: %w", err)
	}

	return nil
}

func removeCronSchedule(timerName string) error {
	cronPath := cronFilePath(timerName)
	if err := os.Remove(cronPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func buildCronCommand(mirrorvaultPath, engine, dbList, compression, secretPath string, hasSecret bool, catchup bool) string {
	script := fmt.Sprintf(
		`MIRRORVAULT_SCHEDULED=true MIRRORVAULT_SCHEDULED_ENGINE="%s" MIRRORVAULT_SCHEDULED_DBS="%s"`,
		engine,
		dbList,
	)
	if catchup {
		script = fmt.Sprintf(
			`MIRRORVAULT_SCHEDULED=true MIRRORVAULT_SCHEDULED_CATCHUP=true MIRRORVAULT_SCHEDULED_ENGINE="%s" MIRRORVAULT_SCHEDULED_DBS="%s"`,
			engine,
			dbList,
		)
	}
	if compression != "" {
		script += fmt.Sprintf(` MV_BACKUP_COMPRESSION="%s"`, compression)
	}

	if hasSecret {
		script += fmt.Sprintf(`; set -a; . "%s"; set +a`, secretPath)
	}

	script += fmt.Sprintf(`; exec "%s" backup`, mirrorvaultPath)

	return fmt.Sprintf(`/bin/sh -c '%s'`, shellEscapeSingleQuotes(script))
}

func shellEscapeSingleQuotes(value string) string {
	return strings.ReplaceAll(value, `'`, `'"'"'`)
}
