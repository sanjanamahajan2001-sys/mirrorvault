package validate

import (
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type DumpInfo struct {
	Format      string // "sql", "mongodb", "redis", "sqlite", "postgres_custom", "postgres_dir", "redis_aof"
	Compressed  bool
	Compression string // "gz", "bz2", "zip"
	Archive     string // "tar", "zip"
	IsMultiDB   bool
	Databases   []string // List of databases in dump (for multi-DB dumps)
	Size        int64
}

// ValidateFormatCompatibility checks if the dump format is compatible with the selected engine
func ValidateFormatCompatibility(dumpInfo *DumpInfo, engine string) error {
	// Map of engines to their compatible formats
	engineFormats := map[string][]string{
		"MySQL":      {"sql"},
		"PostgreSQL": {"sql", "postgres_custom", "postgres_dir"},
		"MongoDB":    {"mongodb"},
		"Redis":      {"redis", "redis_aof"},
		"SQLite":     {"sqlite", "sql"}, // SQLite can restore from both .db files and .sql dumps
		"MSSQL":      {"mssql"},
	}

	compatibleFormats, ok := engineFormats[engine]
	if !ok {
		return fmt.Errorf("unknown engine: %s", engine)
	}

	// Check if dump format is compatible
	for _, format := range compatibleFormats {
		if dumpInfo.Format == format {
			return nil // Format is compatible
		}
	}

	// Format mismatch - provide helpful error message
	formatName := map[string]string{
		"sql":             "SQL dump",
		"mongodb":         "MongoDB dump",
		"redis":           "Redis dump",
		"redis_aof":       "Redis AOF",
		"sqlite":          "SQLite database",
		"postgres_custom": "PostgreSQL custom archive",
		"postgres_dir":    "PostgreSQL directory format",
		"mssql":           "MSSQL backup",
	}[dumpInfo.Format]

	engineName := engine
	if engine == "MySQL" || engine == "PostgreSQL" {
		engineName = engine + " (SQL)"
	}

	return fmt.Errorf(
		"dump format mismatch: the selected dump file is a %s (%s), but you selected %s database engine. "+
			"Please select the correct database engine or use a dump file compatible with %s",
		formatName,
		dumpInfo.Format,
		engineName,
		engineName,
	)
}

// ValidateDump analyzes a dump file and returns information about it
func ValidateDump(dumpPath string) (*DumpInfo, error) {
	info, err := os.Stat(dumpPath)
	if err != nil {
		return nil, fmt.Errorf("dump file not found: %w", err)
	}

	dumpInfo := &DumpInfo{
		Size: info.Size(),
	}

	// Detect compression/archive first
	lowerPath := strings.ToLower(dumpPath)
	ext := strings.ToLower(filepath.Ext(dumpPath))
	compressionExt := ""
	if strings.HasSuffix(lowerPath, ".tar.gz") {
		dumpInfo.Compressed = true
		dumpInfo.Compression = "gz"
		dumpInfo.Archive = "tar"
		compressionExt = ".tar.gz"
	} else if strings.HasSuffix(lowerPath, ".tgz") {
		dumpInfo.Compressed = true
		dumpInfo.Compression = "gz"
		dumpInfo.Archive = "tar"
		compressionExt = ".tgz"
	} else if strings.HasSuffix(lowerPath, ".tar.bz2") {
		dumpInfo.Compressed = true
		dumpInfo.Compression = "bz2"
		dumpInfo.Archive = "tar"
		compressionExt = ".tar.bz2"
	} else if ext == ".tar" {
		dumpInfo.Archive = "tar"
		compressionExt = ".tar"
	} else if ext == ".gz" {
		dumpInfo.Compressed = true
		dumpInfo.Compression = "gz"
		compressionExt = ".gz"
	} else if ext == ".bz2" {
		dumpInfo.Compressed = true
		dumpInfo.Compression = "bz2"
		compressionExt = ".bz2"
	} else if ext == ".zip" {
		dumpInfo.Compressed = true
		dumpInfo.Compression = "zip"
		dumpInfo.Archive = "zip"
		compressionExt = ".zip"
	}

	// Detect format based on extension and content
	// Remove compression extension to get base extension
	basePath := dumpPath
	if compressionExt != "" {
		basePath = strings.TrimSuffix(dumpPath, compressionExt)
	}
	baseExt := strings.ToLower(filepath.Ext(basePath))

	if dumpInfo.Archive != "" {
		format, err := detectFormatFromArchive(dumpPath, dumpInfo)
		if err != nil {
			return nil, fmt.Errorf("could not detect archive format: %w", err)
		}
		dumpInfo.Format = format
	} else if baseExt == ".sql" {
		dumpInfo.Format = "sql"
	} else if baseExt == ".dump" || baseExt == ".backup" {
		dumpInfo.Format = "postgres_custom"
	} else if baseExt == ".bak" {
		dumpInfo.Format = "mssql"
	} else if baseExt == ".rdb" {
		dumpInfo.Format = "redis"
	} else if baseExt == ".aof" || strings.HasSuffix(lowerPath, "appendonly.aof") {
		dumpInfo.Format = "redis_aof"
	} else if baseExt == ".db" || baseExt == ".sqlite" {
		dumpInfo.Format = "sqlite"
	} else if info.IsDir() {
		if isPostgresDirectory(dumpPath) {
			dumpInfo.Format = "postgres_dir"
		} else {
			// MongoDB dumps are typically directories
			dumpInfo.Format = "mongodb"
		}
	} else if baseExt == "" || baseExt == ".archive" || baseExt == ".bson" {
		// MongoDB archive format (single file, possibly compressed)
		// Check content to confirm
		format, err := detectFormatFromContent(dumpPath, dumpInfo.Compressed, dumpInfo.Compression)
		if err == nil && format == "mongodb" {
			dumpInfo.Format = "mongodb"
		} else if baseExt == ".archive" || (baseExt == "" && !dumpInfo.Compressed) {
			// Likely MongoDB archive if no extension or .archive extension
			dumpInfo.Format = "mongodb"
		}
	} else {
		// Try to detect from content
		format, err := detectFormatFromContent(dumpPath, dumpInfo.Compressed, dumpInfo.Compression)
		if err != nil {
			return nil, fmt.Errorf("could not detect dump format: %w", err)
		}
		dumpInfo.Format = format
	}

	// For SQL dumps, check if it's multi-database
	if dumpInfo.Format == "sql" {
		databases, isMulti := detectSQLDatabases(dumpPath, dumpInfo.Compressed, dumpInfo.Compression)
		dumpInfo.IsMultiDB = isMulti
		dumpInfo.Databases = databases
	}

	// For MongoDB dumps, check if it's multi-database (has multiple database subdirectories)
	if dumpInfo.Format == "mongodb" && info.IsDir() {
		entries, err := os.ReadDir(dumpPath)
		if err == nil {
			dbCount := 0
			databases := []string{}
			for _, entry := range entries {
				if entry.IsDir() {
					// Check if directory contains .bson files (MongoDB collection files)
					dbPath := filepath.Join(dumpPath, entry.Name())
					dbEntries, err := os.ReadDir(dbPath)
					if err == nil {
						hasBSON := false
						for _, dbEntry := range dbEntries {
							if !dbEntry.IsDir() && (strings.HasSuffix(dbEntry.Name(), ".bson") || strings.HasSuffix(dbEntry.Name(), ".metadata.json")) {
								hasBSON = true
								break
							}
						}
						if hasBSON {
							dbCount++
							databases = append(databases, entry.Name())
						}
					}
				}
			}
			dumpInfo.IsMultiDB = dbCount > 1
			dumpInfo.Databases = databases
		}
	}

	return dumpInfo, nil
}

func detectFormatFromContent(dumpPath string, compressed bool, compression string) (string, error) {
	var reader io.Reader
	file, err := os.Open(dumpPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	reader = file

	// Handle compression
	if compressed {
		switch compression {
		case "gz":
			gzReader, err := gzip.NewReader(file)
			if err != nil {
				return "", err
			}
			defer gzReader.Close()
			reader = gzReader
		case "bz2":
			reader = bzip2.NewReader(file)
		default:
			return "", fmt.Errorf("unsupported compression: %s", compression)
		}
	}

	// Read first few bytes to detect format
	buf := make([]byte, 512)
	n, err := reader.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}

	content := string(buf[:n])

	// Check for PostgreSQL custom format magic
	if n >= 5 && string(buf[:5]) == "PGDMP" {
		return "postgres_custom", nil
	}

	// Check for MongoDB BSON archive markers (binary format)
	// MongoDB archives start with specific binary markers
	if len(buf) >= 4 {
		// Check for BSON magic bytes or archive format
		// Archive format can be detected by checking for BSON-like structure
		if buf[0] == 0x1F && buf[1] == 0x8B {
			// This is gzip, but could be MongoDB archive
			// We'll let the extension-based detection handle it
		} else if len(buf) >= 16 {
			// Check for BSON document structure (starts with document length as int32)
			// This is a heuristic - BSON documents have length prefix
			docLen := int(buf[0]) | int(buf[1])<<8 | int(buf[2])<<16 | int(buf[3])<<24
			if docLen > 0 && docLen < 100*1024*1024 { // Reasonable document size
				// Could be BSON, but also could be other binary formats
				// We'll be conservative and only return mongodb if we're more certain
			}
		}
	}

	// Check for SQL markers (case-insensitive)
	contentUpper := strings.ToUpper(content)
	if strings.Contains(contentUpper, "CREATE TABLE") || 
	   strings.Contains(contentUpper, "INSERT INTO") ||
	   strings.Contains(contentUpper, "DROP TABLE") ||
	   strings.Contains(contentUpper, "CREATE DATABASE") ||
	   strings.Contains(contentUpper, "USE ") ||
	   strings.Contains(contentUpper, "-- MySQL dump") ||
	   strings.Contains(contentUpper, "-- PostgreSQL database dump") {
		return "sql", nil
	}

	// Check for MongoDB markers
	if strings.Contains(content, "mongodump") || strings.Contains(content, "bson") {
		return "mongodb", nil
	}

	// If we can't detect, but file has no extension or unknown extension,
	// default to SQL for text files (most common case)
	if len(content) > 0 && (content[0] == '-' || content[0] == '/' || content[0] == 'C' || content[0] == 'I') {
		// Looks like SQL dump (starts with comment or SQL keyword)
		return "sql", nil
	}

	return "", fmt.Errorf("unknown dump format")
}

func detectSQLDatabases(dumpPath string, compressed bool, compression string) ([]string, bool) {
	var reader io.Reader
	file, err := os.Open(dumpPath)
	if err != nil {
		return []string{}, false
	}
	defer file.Close()

	reader = file

	// Handle compression
	if compressed {
		switch compression {
		case "gz":
			gzReader, err := gzip.NewReader(file)
			if err != nil {
				return []string{}, false
			}
			defer gzReader.Close()
			reader = gzReader
		case "bz2":
			reader = bzip2.NewReader(file)
		}
	}

	// Read first 64KB to find database names
	buf := make([]byte, 64*1024)
	n, err := reader.Read(buf)
	if err != nil && err != io.EOF {
		return []string{}, false
	}

	content := string(buf[:n])
	databases := make(map[string]bool)

	// Look for CREATE DATABASE statements
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		upper := strings.ToUpper(line)
		
		// MySQL format: CREATE DATABASE `dbname`;
		if strings.HasPrefix(upper, "CREATE DATABASE") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				dbName := strings.Trim(parts[2], "`\"'")
				databases[dbName] = true
			}
		}
		
		// PostgreSQL format: \c dbname or \connect dbname or CREATE DATABASE dbname;
		// Handle both \c and \connect (with and without backslash)
		if strings.HasPrefix(upper, "\\C ") || strings.HasPrefix(upper, "\\CONNECT ") || 
		   strings.HasPrefix(upper, "\\C\t") || strings.HasPrefix(upper, "\\CONNECT\t") {
			// Extract database name - could be after \c or \connect
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				dbName := strings.Trim(parts[1], "`\"'")
				// Remove any trailing semicolons or other characters
				dbName = strings.TrimRight(dbName, ";")
				if dbName != "" {
					databases[dbName] = true
				}
			}
		}
		
		// Also check for "Database \"dbname\" dump" pattern in cluster dumps
		if strings.Contains(upper, "DATABASE") && strings.Contains(upper, "DUMP") {
			// Look for pattern: -- Database "dbname" dump
			if strings.Contains(line, "\"") {
				start := strings.Index(line, "\"")
				end := strings.LastIndex(line, "\"")
				if start < end {
					dbName := line[start+1:end]
					if dbName != "" {
						databases[dbName] = true
					}
				}
			}
		}
	}

	dbList := []string{}
	for db := range databases {
		dbList = append(dbList, db)
	}

	// If no databases found, assume single database dump
	if len(dbList) == 0 {
		return []string{"unknown"}, false
	}

	return dbList, len(dbList) > 1
}

// ExtractDatabaseFromDump extracts a specific database from a multi-database dump
// Returns path to extracted dump file
func ExtractDatabaseFromDump(dumpPath, targetDB string, dumpInfo *DumpInfo) (string, error) {
	if !dumpInfo.IsMultiDB {
		// Single database dump, return original path
		return dumpPath, nil
	}

	// Create temporary file for extracted database
	tmpFile, err := os.CreateTemp("", fmt.Sprintf("mirrorvault_restore_%s_*.sql", targetDB))
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	reader, closeReader, err := OpenDecompressedReader(dumpPath, dumpInfo)
	if err != nil {
		return "", err
	}
	defer closeReader()

	// Write extracted database to temp file
	outFile, err := os.Create(tmpPath)
	if err != nil {
		return "", err
	}
	defer outFile.Close()

	// For compressed output, we'll write uncompressed for now
	// In production, you might want to keep it compressed
	inTargetDB := false
	foundTargetDB := false
	buf := make([]byte, 64*1024)
	lineBuffer := "" // Buffer for incomplete lines across chunks
	
	for {
		n, err := reader.Read(buf)
		if n == 0 && err == io.EOF {
			// Process remaining buffer
			if lineBuffer != "" {
				if err := processLine(lineBuffer, targetDB, &inTargetDB, &foundTargetDB, outFile); err != nil {
					return "", err
				}
			}
			break
		}
		if n == 0 {
			break
		}
		if err != nil && err != io.EOF {
			// If we've found and started extracting the target DB, try to continue
			// (gzip might be slightly corrupted but still readable)
			if foundTargetDB {
				// Process remaining buffer and break
				if lineBuffer != "" {
					if err := processLine(lineBuffer, targetDB, &inTargetDB, &foundTargetDB, outFile); err != nil {
						return "", err
					}
				}
				break
			}
			return "", fmt.Errorf("failed to read dump file (may be corrupted): %w", err)
		}

		chunk := lineBuffer + string(buf[:n])
		lines := strings.Split(chunk, "\n")
		
		// Last line might be incomplete, save it for next iteration
		lineBuffer = lines[len(lines)-1]
		lines = lines[:len(lines)-1]
		
		for _, line := range lines {
			if err := processLine(line, targetDB, &inTargetDB, &foundTargetDB, outFile); err != nil {
				return "", err
			}
		}

		if err == io.EOF {
			// Process remaining buffer
			if lineBuffer != "" {
				if err := processLine(lineBuffer, targetDB, &inTargetDB, &foundTargetDB, outFile); err != nil {
					return "", err
				}
			}
			break
		}
	}

	if !foundTargetDB {
		return "", fmt.Errorf("target database '%s' not found in cluster dump", targetDB)
	}

	// Verify extracted file has content
	info, err := os.Stat(tmpPath)
	if err != nil {
		return "", fmt.Errorf("failed to stat extracted dump file: %w", err)
	}
	if info.Size() == 0 {
		return "", fmt.Errorf("extracted database dump is empty - target database '%s' may not exist in cluster dump", targetDB)
	}

	return tmpPath, nil
}

// Helper function to process a line and determine if we're in the target database
func processLine(line, targetDB string, inTargetDB *bool, foundTargetDB *bool, outFile *os.File) error {
	upper := strings.ToUpper(strings.TrimSpace(line))
	
	// Detect database boundaries for PostgreSQL cluster dumps
	// PostgreSQL uses: \connect dbname or \c dbname
	if strings.HasPrefix(upper, "\\CONNECT ") || strings.HasPrefix(upper, "\\C ") ||
	   strings.HasPrefix(upper, "\\CONNECT\t") || strings.HasPrefix(upper, "\\C\t") {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			dbName := strings.Trim(parts[1], "`\"';")
			dbName = strings.TrimRight(dbName, ";")
			*inTargetDB = (dbName == targetDB)
			if *inTargetDB {
				*foundTargetDB = true
				if _, err := outFile.WriteString(line + "\n"); err != nil {
					return err
				}
			}
			return nil
		}
	}
	
	// Also check for MySQL format: CREATE DATABASE or USE
	if strings.HasPrefix(upper, "CREATE DATABASE") || strings.HasPrefix(upper, "USE ") {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			dbName := strings.Trim(parts[len(parts)-1], "`\"';")
			*inTargetDB = (dbName == targetDB)
			if *inTargetDB {
				*foundTargetDB = true
				if _, err := outFile.WriteString(line + "\n"); err != nil {
					return err
				}
			}
			return nil
		}
	}
	
	// Check for "Database \"dbname\" dump" header in cluster dumps
	if strings.Contains(upper, "DATABASE") && strings.Contains(upper, "DUMP") {
		if strings.Contains(line, "\"") {
			start := strings.Index(line, "\"")
			end := strings.LastIndex(line, "\"")
			if start < end {
				dbName := line[start+1:end]
				*inTargetDB = (dbName == targetDB)
				if *inTargetDB {
					*foundTargetDB = true
					if _, err := outFile.WriteString(line + "\n"); err != nil {
						return err
					}
				}
				return nil
			}
		}
	}
	
	// Write line if we're in target database
	if *inTargetDB {
		if _, err := outFile.WriteString(line + "\n"); err != nil {
			return err
		}
	}
	
	return nil
}
