package schedule

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const credentialsDir = "/var/lib/mirrorvault/credentials"

func secretFilePath(timerName string) string {
	baseName := strings.TrimSuffix(timerName, ".timer")
	return filepath.Join(credentialsDir, fmt.Sprintf("%s.env", baseName))
}

func writeSecretFile(timerName, engine, password string) (string, error) {
	if password == "" {
		_ = removeSecretFile(timerName)
		return "", nil
	}

	if err := os.MkdirAll(credentialsDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create credentials directory: %w", err)
	}

	secretPath := secretFilePath(timerName)
	content := fmt.Sprintf("MIRRORVAULT_%s_PASSWORD=%s\n", strings.ToUpper(engine), password)
	if err := os.WriteFile(secretPath, []byte(content), 0600); err != nil {
		return "", fmt.Errorf("failed to write credentials file: %w", err)
	}

	return secretPath, nil
}

func removeSecretFile(timerName string) error {
	secretPath := secretFilePath(timerName)
	if err := os.Remove(secretPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func renameSecretFile(oldTimerName, newTimerName string) error {
	oldPath := secretFilePath(oldTimerName)
	newPath := secretFilePath(newTimerName)

	if _, err := os.Stat(oldPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if err := os.Rename(oldPath, newPath); err != nil {
		return err
	}

	return nil
}

func secretFileExists(timerName string) (string, bool) {
	secretPath := secretFilePath(timerName)
	if _, err := os.Stat(secretPath); err == nil {
		return secretPath, true
	}
	return "", false
}
