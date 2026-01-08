package detect

import (
	"bytes"
	"os/exec"
	"strings"
)

// CommandExists checks if a command exists in PATH
func CommandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

// execOutput runs a command and returns trimmed stdout
func execOutput(cmd string, args ...string) (string, error) {
	c := exec.Command(cmd, args...)
	var out bytes.Buffer
	c.Stdout = &out
	c.Stderr = &out

	if err := c.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

// normalizeVersion cleans noisy CLI version output
func normalizeVersion(raw string) string {
	raw = strings.TrimSpace(raw)

	// MySQL / MariaDB
	if strings.Contains(raw, "Distrib") || strings.Contains(raw, "Ver") {
		for _, f := range strings.Fields(raw) {
			if strings.Count(f, ".") >= 1 && f[0] >= '0' && f[0] <= '9' {
				return f
			}
		}
	}

	// PostgreSQL
	if strings.HasPrefix(raw, "psql") {
		parts := strings.Fields(raw)
		if len(parts) >= 3 {
			return parts[2]
		}
	}

	// Redis
	if strings.Contains(raw, "Redis server") {
		for _, p := range strings.Fields(raw) {
			if strings.HasPrefix(p, "v=") {
				return strings.TrimPrefix(p, "v=")
			}
		}
	}

	// SQLite
	if strings.Count(raw, ".") >= 1 {
		return strings.Fields(raw)[0]
	}

	return raw
}
