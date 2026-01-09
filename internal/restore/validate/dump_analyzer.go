package validate

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strings"
)

// ExtractTablesFromDump extracts table names from a SQL dump file
func ExtractTablesFromDump(dumpPath string, compressed bool, compression string) ([]string, error) {
	var reader io.Reader
	file, err := os.Open(dumpPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open dump file: %w", err)
	}
	defer file.Close()

	reader = file

	// Handle compression
	if compressed {
		switch compression {
		case "gz":
			// Check file size first
			fileInfo, err := file.Stat()
			if err == nil && fileInfo.Size() < 2 {
				// File too small, return empty list (restore will handle it)
				return []string{}, nil
			}
			
			// Check if file is actually gzipped by reading header
			header := make([]byte, 2)
			n, err := file.Read(header)
			if err != nil && err != io.EOF {
				// Can't read header, return empty list (restore will handle it)
				return []string{}, nil
			}
			if n < 2 {
				// File too small, return empty list (restore will handle it)
				return []string{}, nil
			}
			if _, err := file.Seek(0, 0); err != nil {
				// Can't reset, return empty list (restore will handle it)
				return []string{}, nil
			}
			
			// Check gzip magic number (0x1f 0x8b)
			if header[0] != 0x1f || header[1] != 0x8b {
				// Not a gzip file, return empty list (restore will handle it)
				return []string{}, nil
			}
			
			gzReader, err := gzip.NewReader(file)
			if err != nil {
				// Can't create gzip reader, return empty list (restore will handle it)
				return []string{}, nil
			}
			defer gzReader.Close()
			reader = gzReader
		}
	}

	// Read the entire file (or first 10MB) to find table names
	// For large files, we'll read in chunks
	tables := make(map[string]bool)
	buf := make([]byte, 64*1024) // 64KB chunks
	totalRead := 0
	maxRead := 10 * 1024 * 1024 // 10MB max

	for totalRead < maxRead {
		n, err := reader.Read(buf)
		if n == 0 {
			break
		}
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("failed to read dump file: %w", err)
		}

		content := string(buf[:n])
		lines := strings.Split(content, "\n")
		
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			upper := strings.ToUpper(line)
			
			// MySQL/PostgreSQL format: CREATE TABLE `tablename` or CREATE TABLE tablename
			if strings.HasPrefix(upper, "CREATE TABLE") {
				// Extract table name - handle both `table` and table formats
				// CREATE TABLE `table_name` ( or CREATE TABLE table_name (
				parts := strings.Fields(line)
				if len(parts) >= 3 {
					tableName := parts[2]
					// Remove backticks, quotes
					tableName = strings.Trim(tableName, "`\"'")
					// Remove any trailing characters like ( or ;
					tableName = strings.TrimRight(tableName, "(;")
					if tableName != "" {
						tables[tableName] = true
					}
				}
			}
			
			// Also check for INSERT INTO statements to find tables
			if strings.HasPrefix(upper, "INSERT INTO") {
				parts := strings.Fields(line)
				if len(parts) >= 3 {
					tableName := parts[2]
					tableName = strings.Trim(tableName, "`\"'")
					tableName = strings.TrimRight(tableName, "(;")
					if tableName != "" {
						tables[tableName] = true
					}
				}
			}
			
			// Check for DROP TABLE statements (indicates table names)
			if strings.HasPrefix(upper, "DROP TABLE") {
				parts := strings.Fields(line)
				if len(parts) >= 3 {
					tableName := parts[2]
					tableName = strings.Trim(tableName, "`\"'")
					tableName = strings.TrimRight(tableName, "(;")
					if tableName != "" {
						tables[tableName] = true
					}
				}
			}
		}

		totalRead += n
		if err == io.EOF {
			break
		}
	}

	tableList := []string{}
	for table := range tables {
		if table != "" {
			tableList = append(tableList, table)
		}
	}

	return tableList, nil
}
