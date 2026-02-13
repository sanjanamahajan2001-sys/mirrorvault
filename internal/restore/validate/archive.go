package validate

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func detectFormatFromArchive(dumpPath string, dumpInfo *DumpInfo) (string, error) {
	entries, err := listArchiveEntries(dumpPath, dumpInfo)
	if err != nil {
		return "", err
	}
	if format := detectFormatFromArchiveEntries(entries); format != "" {
		return format, nil
	}
	return "", fmt.Errorf("unknown dump format in archive")
}

func ExtractArchiveIfNeeded(dumpPath string, dumpInfo *DumpInfo) (string, func(), error) {
	if dumpInfo.Archive == "" {
		return dumpPath, func() {}, nil
	}

	tempDir, err := os.MkdirTemp("", "mirrorvault_extract_*")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() {
		_ = os.RemoveAll(tempDir)
	}

	switch dumpInfo.Archive {
	case "zip":
		if err := extractZip(dumpPath, tempDir); err != nil {
			cleanup()
			return "", func() {}, err
		}
	case "tar":
		if err := extractTar(dumpPath, dumpInfo, tempDir); err != nil {
			cleanup()
			return "", func() {}, err
		}
	default:
		cleanup()
		return "", func() {}, fmt.Errorf("unsupported archive type: %s", dumpInfo.Archive)
	}

	targetPath, err := findBestExtractedPath(tempDir, dumpInfo)
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	return targetPath, cleanup, nil
}

func isPostgresDirectory(path string) bool {
	if _, err := os.Stat(filepath.Join(path, "toc.dat")); err == nil {
		return true
	}
	return false
}

func listArchiveEntries(dumpPath string, dumpInfo *DumpInfo) ([]string, error) {
	switch dumpInfo.Archive {
	case "zip":
		zipReader, err := zip.OpenReader(dumpPath)
		if err != nil {
			return nil, err
		}
		defer zipReader.Close()
		entries := make([]string, 0, len(zipReader.File))
		for _, f := range zipReader.File {
			entries = append(entries, f.Name)
		}
		return entries, nil
	case "tar":
		reader, closeReader, err := openTarReader(dumpPath, dumpInfo)
		if err != nil {
			return nil, err
		}
		defer closeReader()
		tarReader := tar.NewReader(reader)
		entries := []string{}
		for {
			hdr, err := tarReader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			entries = append(entries, hdr.Name)
		}
		return entries, nil
	default:
		return nil, fmt.Errorf("unsupported archive type: %s", dumpInfo.Archive)
	}
}

func detectFormatFromArchiveEntries(entries []string) string {
	for _, name := range entries {
		lower := strings.ToLower(name)
		switch {
		case strings.HasSuffix(lower, ".bson") || strings.HasSuffix(lower, ".metadata.json"):
			return "mongodb"
		case strings.HasSuffix(lower, "toc.dat"):
			return "postgres_dir"
		case strings.HasSuffix(lower, ".sql"):
			return "sql"
		case strings.HasSuffix(lower, ".rdb"):
			return "redis"
		case strings.HasSuffix(lower, ".aof") || strings.HasSuffix(lower, "appendonly.aof"):
			return "redis_aof"
		case strings.HasSuffix(lower, ".bak"):
			return "mssql"
		case strings.HasSuffix(lower, ".db") || strings.HasSuffix(lower, ".sqlite"):
			return "sqlite"
		case strings.HasSuffix(lower, ".dump") || strings.HasSuffix(lower, ".backup"):
			return "postgres_custom"
		case strings.HasSuffix(lower, ".archive"):
			return "mongodb"
		}
	}
	return ""
}

func openTarReader(dumpPath string, dumpInfo *DumpInfo) (io.Reader, func() error, error) {
	file, err := os.Open(dumpPath)
	if err != nil {
		return nil, func() error { return nil }, err
	}

	reader := io.Reader(file)
	if dumpInfo.Compressed {
		switch dumpInfo.Compression {
		case "gz":
			gzReader, err := gzip.NewReader(file)
			if err != nil {
				_ = file.Close()
				return nil, func() error { return nil }, err
			}
			return gzReader, func() error {
				_ = gzReader.Close()
				return file.Close()
			}, nil
		case "bz2":
			reader = bzip2.NewReader(file)
		default:
			_ = file.Close()
			return nil, func() error { return nil }, fmt.Errorf("unsupported tar compression: %s", dumpInfo.Compression)
		}
	}

	return reader, file.Close, nil
}

func extractZip(path, dest string) error {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, f := range reader.File {
		targetPath, err := safeJoin(dest, f.Name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}
		src, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(targetPath)
		if err != nil {
			src.Close()
			return err
		}
		if _, err := io.Copy(out, src); err != nil {
			src.Close()
			out.Close()
			return err
		}
		src.Close()
		out.Close()
	}
	return nil
}

func extractTar(path string, dumpInfo *DumpInfo, dest string) error {
	reader, closeReader, err := openTarReader(path, dumpInfo)
	if err != nil {
		return err
	}
	defer closeReader()

	tarReader := tar.NewReader(reader)
	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		targetPath, err := safeJoin(dest, hdr.Name)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return err
			}
			out, err := os.Create(targetPath)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tarReader); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}

func findBestExtractedPath(root string, dumpInfo *DumpInfo) (string, error) {
	switch dumpInfo.Format {
	case "mongodb":
		if dir, ok := findMongoDumpRoot(root); ok {
			return dir, nil
		}
	case "postgres_dir":
		if dir, ok := findDirectoryWithFile(root, "toc.dat"); ok {
			return dir, nil
		}
	case "sql":
		if file, ok := findFirstMatchingFile(root, func(name string) bool {
			return strings.HasSuffix(strings.ToLower(name), ".sql")
		}); ok {
			return file, nil
		}
	case "postgres_custom":
		if file, ok := findFirstMatchingFile(root, func(name string) bool {
			lower := strings.ToLower(name)
			return strings.HasSuffix(lower, ".dump") || strings.HasSuffix(lower, ".backup")
		}); ok {
			return file, nil
		}
	case "redis":
		if file, ok := findFirstMatchingFile(root, func(name string) bool {
			return strings.HasSuffix(strings.ToLower(name), ".rdb")
		}); ok {
			return file, nil
		}
	case "redis_aof":
		if file, ok := findFirstMatchingFile(root, func(name string) bool {
			lower := strings.ToLower(name)
			return strings.HasSuffix(lower, ".aof") || strings.HasSuffix(lower, "appendonly.aof")
		}); ok {
			return file, nil
		}
	case "mssql":
		if file, ok := findFirstMatchingFile(root, func(name string) bool {
			return strings.HasSuffix(strings.ToLower(name), ".bak")
		}); ok {
			return file, nil
		}
	case "sqlite":
		if file, ok := findFirstMatchingFile(root, func(name string) bool {
			lower := strings.ToLower(name)
			return strings.HasSuffix(lower, ".db") || strings.HasSuffix(lower, ".sqlite")
		}); ok {
			return file, nil
		}
	}

	if file, ok := findFirstMatchingFile(root, func(name string) bool { return true }); ok {
		return file, nil
	}

	return "", fmt.Errorf("no compatible dump found in extracted archive")
}

func findMongoDumpRoot(root string) (string, bool) {
	found := ""
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if !entry.IsDir() {
			return nil
		}
		if hasMongoDatabaseSubdirs(path) {
			found = path
			return filepath.SkipDir
		}
		return nil
	})
	if found != "" {
		return found, true
	}
	return "", false
}

func hasMongoDatabaseSubdirs(path string) bool {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if hasMongoFiles(filepath.Join(path, entry.Name())) {
			return true
		}
	}
	return false
}

func hasMongoFiles(path string) bool {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		lower := strings.ToLower(entry.Name())
		if strings.HasSuffix(lower, ".bson") || strings.HasSuffix(lower, ".metadata.json") {
			return true
		}
	}
	return false
}

func findDirectoryWithFile(root, filename string) (string, bool) {
	found := ""
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if !entry.IsDir() {
			return nil
		}
		if _, err := os.Stat(filepath.Join(path, filename)); err == nil {
			found = path
			return filepath.SkipDir
		}
		return nil
	})
	if found != "" {
		return found, true
	}
	return "", false
}

func findFirstMatchingFile(root string, match func(name string) bool) (string, bool) {
	var found string
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if match(path) {
			found = path
			return nil
		}
		return nil
	})
	if found != "" {
		return found, true
	}
	return "", false
}

func safeJoin(base, name string) (string, error) {
	cleaned := filepath.Clean(name)
	if filepath.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid archive path: %s", name)
	}
	return filepath.Join(base, cleaned), nil
}
