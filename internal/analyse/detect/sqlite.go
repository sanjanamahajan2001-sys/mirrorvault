package detect

import (
	"os"
	"path/filepath"
	"strings"

	"mirrorvault/pkg/model"
)

func DetectSQLite() *model.Database {
	if !CommandExists("sqlite3") {
		return nil
	}

	var files []string
	searchRoots := []string{"/home", "/var/lib", "/opt"}

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
			if err != nil || d.IsDir() {
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
