package schedule

import (
	"os"
	"os/exec"
)

type backendType string

const (
	backendSystemd backendType = "systemd"
	backendCron    backendType = "cron"
	backendNone    backendType = "none"
)

func detectBackend() backendType {
	if hasSystemd() {
		return backendSystemd
	}
	if hasCron() {
		return backendCron
	}
	return backendNone
}

func hasSystemd() bool {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false
	}
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		return true
	}
	return false
}

func hasCron() bool {
	if _, err := exec.LookPath("crontab"); err != nil {
		return false
	}
	if _, err := os.Stat("/etc/cron.d"); err == nil {
		return true
	}
	return false
}
