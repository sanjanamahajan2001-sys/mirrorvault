package main

import (
	"fmt"
	"os"
	"strings"

	"mirrorvault/internal/backup/credentials"
	"mirrorvault/internal/backup/plan"
	"mirrorvault/pkg/model"

	"golang.org/x/term"
)

type stringSlice []string

func (s *stringSlice) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSlice) Set(value string) error {
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			*s = append(*s, part)
		}
	}
	return nil
}

func parseEngines(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	var engines []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			engines = append(engines, part)
		}
	}
	return engines
}

func buildSelection(scan model.ScanResult, engines []string, dbs []string, all bool) (map[string][]string, error) {
	selection := make(map[string][]string)
	engineMap := make(map[string]model.Database)
	for _, db := range scan.Databases {
		engineMap[db.Engine] = db
	}

	if all {
		if len(engines) == 0 {
			for _, db := range scan.Databases {
				if len(db.Names) > 0 {
					selection[db.Engine] = []string{plan.AllDatabasesName}
				}
			}
			return selection, nil
		}

		for _, engine := range engines {
			db, ok := engineMap[engine]
			if !ok {
				return nil, fmt.Errorf("engine not found: %s", engine)
			}
			if len(db.Names) > 0 {
				selection[engine] = []string{plan.AllDatabasesName}
			}
		}
		return selection, nil
	}

	if len(engines) == 0 {
		return nil, fmt.Errorf("engine is required when selecting specific databases")
	}
	if len(dbs) == 0 {
		return nil, fmt.Errorf("no databases specified")
	}
	if len(engines) > 1 {
		return nil, fmt.Errorf("multiple engines not supported with explicit database list")
	}

	engine := engines[0]
	db, ok := engineMap[engine]
	if !ok {
		return nil, fmt.Errorf("engine not found: %s", engine)
	}

	available := make(map[string]bool)
	for _, name := range db.Names {
		available[name] = true
	}
	for _, name := range dbs {
		if !available[name] {
			return nil, fmt.Errorf("database not found for %s: %s", engine, name)
		}
	}

	selection[engine] = append(selection[engine], dbs...)
	return selection, nil
}

func resolvePassword(engine, passwordFlag, passwordFile string) (string, error) {
	if passwordFlag != "" {
		return passwordFlag, nil
	}
	if passwordFile != "" {
		data, err := os.ReadFile(passwordFile)
		if err != nil {
			return "", fmt.Errorf("failed to read password file: %w", err)
		}
		password := strings.TrimSpace(string(data))
		if password == "" {
			return "", fmt.Errorf("password file is empty")
		}
		return password, nil
	}

	envVar := fmt.Sprintf("MIRRORVAULT_%s_PASSWORD", strings.ToUpper(engine))
	if password := os.Getenv(envVar); password != "" {
		return password, nil
	}

	if term.IsTerminal(int(os.Stdin.Fd())) {
		return credentials.Prompt(engine)
	}

	return "", fmt.Errorf("password required for %s but not provided (use --password, --password-file, or %s)", engine, envVar)
}
