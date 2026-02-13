package logrotate

import (
	"fmt"
	"os"
	"os/exec"
)

const (
	logrotateTarget = "/etc/logrotate.d/mirrorvault"
)

func Install() error {
	content := `/var/log/mirrorvault/*.log {
  daily
  rotate 14
  compress
  missingok
  notifempty
  copytruncate
}
`

	if err := os.WriteFile(logrotateTarget, []byte(content), 0644); err == nil {
		return nil
	}

	tmpFile, err := os.CreateTemp("", "mirrorvault-logrotate-*.conf")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}

	if err := exec.Command("sudo", "mv", tmpFile.Name(), logrotateTarget).Run(); err != nil {
		return fmt.Errorf("failed to install logrotate config: %w", err)
	}
	if err := exec.Command("sudo", "chmod", "0644", logrotateTarget).Run(); err != nil {
		return fmt.Errorf("failed to set logrotate permissions: %w", err)
	}

	return nil
}

