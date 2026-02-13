package detect

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"mirrorvault/pkg/model"
)

func DetectSQLite() *model.Database {
	if !CommandExists("sqlite3") {
		return nil
	}

	var files []string
	searchRoots := sqliteScanRoots()
	maxDepth := sqliteScanMaxDepth()

	// System database paths that are typically locked or not useful to backup
	systemPaths := []string{
		"/var/lib/docker",
		"/var/lib/containerd",
		"/var/lib/PackageKit",
		"/var/lib/command-not-found",
		"/var/lib/.cache",
	}

	for _, root := range searchRoots {
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}

			if d.IsDir() {
				if maxDepth >= 0 {
					if depth, err := pathDepth(root, path); err == nil && depth > maxDepth {
						return filepath.SkipDir
					}
				}
				return nil
			}

			// Skip system databases that are typically locked
			isSystemDB := false
			for _, sysPath := range systemPaths {
				if strings.HasPrefix(path, sysPath) {
					isSystemDB = true
					break
				}
			}
			// Also skip cache databases (often locked)
			if strings.Contains(path, "/.cache/") {
				isSystemDB = true
			}
			if isSystemDB {
				return nil
			}

			if strings.HasSuffix(path, ".db") ||
				strings.HasSuffix(path, ".sqlite") ||
				strings.HasSuffix(path, ".sqlite3") {
				files = append(files, path)
			}
			return nil
		})
	}

	versionRaw, _ := execOutput("sqlite3", "--version")

	return &model.Database{
		Engine:       "SQLite",
		Version:      normalizeVersion(versionRaw),
		Type:         model.SQL,
		RequiresAuth: false,
		Names:        files,
	}
}

func sqliteScanRoots() []string {
	if roots := os.Getenv("MV_SQLITE_SCAN_ROOTS"); roots != "" {
		parts := strings.Split(roots, ":")
		var cleaned []string
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				cleaned = append(cleaned, part)
			}
		}
		if len(cleaned) > 0 {
			return cleaned
		}
	}
	return []string{"/home", "/var/lib", "/opt"}
}

func sqliteScanMaxDepth() int {
	if raw := os.Getenv("MV_SQLITE_MAX_DEPTH"); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil {
			return value
		}
	}
	return -1
}

func pathDepth(root, path string) (int, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return 0, err
	}
	if rel == "." {
		return 0, nil
	}
	return strings.Count(rel, string(os.PathSeparator)) + 1, nil
}
