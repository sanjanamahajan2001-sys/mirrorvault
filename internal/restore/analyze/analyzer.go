package analyze

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type DatabaseStats struct {
	TableCount int
	Tables     []TableStats
	TotalRows  int64
	Size       int64 // Database size in bytes
}

type TableStats struct {
	Name      string
	RowCount  int64
	Size      int64
	Columns   []ColumnInfo
	SampleRows []map[string]string // Sample rows (last 5-10 rows)
}

type ColumnInfo struct {
	Name     string
	Type     string
	Nullable bool
}

// AnalyzeDatabase extracts statistics from a database
func AnalyzeDatabase(engine, database string, requiresAuth bool, password string) (*DatabaseStats, error) {
	switch engine {
	case "MySQL":
		return analyzeMySQL(database, requiresAuth, password)
	case "PostgreSQL":
		return analyzePostgreSQL(database, requiresAuth, password)
	case "MongoDB":
		return analyzeMongoDB(database, requiresAuth, password)
	case "SQLite":
		return analyzeSQLite(database)
	case "Redis":
		return analyzeRedis(database, requiresAuth, password)
	default:
		return nil, fmt.Errorf("analysis not implemented for engine: %s", engine)
	}
}

func analyzeMySQL(database string, requiresAuth bool, password string) (*DatabaseStats, error) {
	stats := &DatabaseStats{
		Tables: []TableStats{},
	}

	// Get list of tables
	var cmd *exec.Cmd
	if requiresAuth {
		cmd = exec.Command("sudo", "mysql", "-u", "root", "-p"+password, "-N", "-e", fmt.Sprintf("USE %s; SHOW TABLES;", database))
	} else {
		cmd = exec.Command("sudo", "mysql", "-u", "root", "-N", "-e", fmt.Sprintf("USE %s; SHOW TABLES;", database))
	}

	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to list tables: %w", err)
	}

	tableNames := []string{}
	for _, line := range strings.Split(out.String(), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			tableNames = append(tableNames, line)
		}
	}

	stats.TableCount = len(tableNames)

	// Get row count and size for each table
	for _, tableName := range tableNames {
		// Get row count
		var countCmd *exec.Cmd
		if requiresAuth {
			countCmd = exec.Command("sudo", "mysql", "-u", "root", "-p"+password, "-N", "-e", fmt.Sprintf("USE %s; SELECT COUNT(*) FROM %s;", database, tableName))
		} else {
			countCmd = exec.Command("sudo", "mysql", "-u", "root", "-N", "-e", fmt.Sprintf("USE %s; SELECT COUNT(*) FROM %s;", database, tableName))
		}

		var countOut bytes.Buffer
		countCmd.Stdout = &countOut
		if err := countCmd.Run(); err != nil {
			// If count fails, just set to 0
			stats.Tables = append(stats.Tables, TableStats{
				Name:     tableName,
				RowCount: 0,
			})
			continue
		}

		countStr := strings.TrimSpace(countOut.String())
		rowCount, _ := strconv.ParseInt(countStr, 10, 64)

		// Get table size
		var sizeCmd *exec.Cmd
		if requiresAuth {
			sizeCmd = exec.Command("sudo", "mysql", "-u", "root", "-p"+password, "-N", "-e", fmt.Sprintf("USE %s; SELECT ROUND(((data_length + index_length) / 1024 / 1024), 2) AS 'Size (MB)' FROM information_schema.TABLES WHERE table_schema = '%s' AND table_name = '%s';", database, database, tableName))
		} else {
			sizeCmd = exec.Command("sudo", "mysql", "-u", "root", "-N", "-e", fmt.Sprintf("USE %s; SELECT ROUND(((data_length + index_length) / 1024 / 1024), 2) AS 'Size (MB)' FROM information_schema.TABLES WHERE table_schema = '%s' AND table_name = '%s';", database, database, tableName))
		}

		var sizeOut bytes.Buffer
		sizeCmd.Stdout = &sizeOut
		var sizeMB float64
		if err := sizeCmd.Run(); err == nil {
			sizeStr := strings.TrimSpace(sizeOut.String())
			sizeMB, _ = strconv.ParseFloat(sizeStr, 64)
		}

		// Get column information
		var colCmd *exec.Cmd
		if requiresAuth {
			colCmd = exec.Command("sudo", "mysql", "-u", "root", "-p"+password, "-N", "-e", fmt.Sprintf("USE %s; DESCRIBE %s;", database, tableName))
		} else {
			colCmd = exec.Command("sudo", "mysql", "-u", "root", "-N", "-e", fmt.Sprintf("USE %s; DESCRIBE %s;", database, tableName))
		}

		var colOut bytes.Buffer
		colCmd.Stdout = &colOut
		columns := []ColumnInfo{}
		if err := colCmd.Run(); err == nil {
			for _, line := range strings.Split(colOut.String(), "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					nullable := "YES"
					if len(parts) >= 3 {
						nullable = parts[2]
					}
					columns = append(columns, ColumnInfo{
						Name:     parts[0],
						Type:     parts[1],
						Nullable: nullable == "YES",
					})
				}
			}
		}

		// Get sample rows (last 5-10 rows)
		sampleRows := []map[string]string{}
		if rowCount > 0 {
			limit := int64(10)
			if rowCount < limit {
				limit = rowCount
			}
			
			// Get column names first
			colNames := []string{}
			for _, col := range columns {
				colNames = append(colNames, col.Name)
			}
			
			if len(colNames) > 0 {
				// Get last N rows - use first column for ordering, or just get last rows
				var sampleCmd *exec.Cmd
				// Try to get rows ordered by first column descending, fallback to simple SELECT
				query := fmt.Sprintf("SELECT * FROM %s LIMIT %d", tableName, limit)
				// If we have a primary key or first column, try to order by it
				if len(colNames) > 0 {
					// Try ordering by first column (usually ID or primary key)
					query = fmt.Sprintf("SELECT * FROM %s ORDER BY %s DESC LIMIT %d", tableName, colNames[0], limit)
				}
				
				if requiresAuth {
					sampleCmd = exec.Command("sudo", "mysql", "-u", "root", "-p"+password, "-N", "-e", fmt.Sprintf("USE %s; %s;", database, query))
				} else {
					sampleCmd = exec.Command("sudo", "mysql", "-u", "root", "-N", "-e", fmt.Sprintf("USE %s; %s;", database, query))
				}

				var sampleOut bytes.Buffer
				sampleCmd.Stdout = &sampleOut
				if err := sampleCmd.Run(); err == nil {
					lines := strings.Split(strings.TrimSpace(sampleOut.String()), "\n")
					for _, line := range lines {
						line = strings.TrimSpace(line)
						if line == "" {
							continue
						}
						// MySQL -N flag outputs tab-separated values
						values := strings.Split(line, "\t")
						row := make(map[string]string)
						for i, val := range values {
							if i < len(colNames) {
								// Truncate long values for display
								if len(val) > 50 {
									val = val[:47] + "..."
								}
								row[colNames[i]] = val
							}
						}
						if len(row) > 0 {
							sampleRows = append(sampleRows, row)
						}
					}
					// Reverse to show in original order (since we used DESC)
					for i, j := 0, len(sampleRows)-1; i < j; i, j = i+1, j-1 {
						sampleRows[i], sampleRows[j] = sampleRows[j], sampleRows[i]
					}
				}
			}
		}

		tableStats := TableStats{
			Name:      tableName,
			RowCount:  rowCount,
			Size:      int64(sizeMB * 1024 * 1024), // Convert MB to bytes
			Columns:   columns,
			SampleRows: sampleRows,
		}

		stats.Tables = append(stats.Tables, tableStats)
		stats.TotalRows += rowCount
		stats.Size += tableStats.Size
	}

	return stats, nil
}

func analyzePostgreSQL(database string, requiresAuth bool, password string) (*DatabaseStats, error) {
	stats := &DatabaseStats{
		Tables: []TableStats{},
	}

	// Get list of tables
	var cmd *exec.Cmd
	if requiresAuth {
		cmd = exec.Command("sudo", "-u", "postgres", "psql", "-d", database, "-t", "-c", "SELECT tablename FROM pg_tables WHERE schemaname = 'public';")
		cmd.Env = append(cmd.Env, fmt.Sprintf("PGPASSWORD=%s", password))
	} else {
		cmd = exec.Command("sudo", "-u", "postgres", "psql", "-d", database, "-t", "-c", "SELECT tablename FROM pg_tables WHERE schemaname = 'public';")
	}

	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to list tables: %w", err)
	}

	tableNames := []string{}
	for _, line := range strings.Split(out.String(), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			tableNames = append(tableNames, line)
		}
	}

	stats.TableCount = len(tableNames)

	// Get row count and size for each table
	for _, tableName := range tableNames {
		// Get row count
		var countCmd *exec.Cmd
		if requiresAuth {
			countCmd = exec.Command("sudo", "-u", "postgres", "psql", "-d", database, "-t", "-c", fmt.Sprintf("SELECT COUNT(*) FROM %s;", tableName))
			countCmd.Env = append(countCmd.Env, fmt.Sprintf("PGPASSWORD=%s", password))
		} else {
			countCmd = exec.Command("sudo", "-u", "postgres", "psql", "-d", database, "-t", "-c", fmt.Sprintf("SELECT COUNT(*) FROM %s;", tableName))
		}

		var countOut bytes.Buffer
		countCmd.Stdout = &countOut
		rowCount := int64(0)
		if err := countCmd.Run(); err == nil {
			countStr := strings.TrimSpace(countOut.String())
			rowCount, _ = strconv.ParseInt(countStr, 10, 64)
		}

		// Get table size
		var sizeCmd *exec.Cmd
		if requiresAuth {
			sizeCmd = exec.Command("sudo", "-u", "postgres", "psql", "-d", database, "-t", "-c", fmt.Sprintf("SELECT pg_total_relation_size('%s');", tableName))
			sizeCmd.Env = append(sizeCmd.Env, fmt.Sprintf("PGPASSWORD=%s", password))
		} else {
			sizeCmd = exec.Command("sudo", "-u", "postgres", "psql", "-d", database, "-t", "-c", fmt.Sprintf("SELECT pg_total_relation_size('%s');", tableName))
		}

		var sizeOut bytes.Buffer
		sizeCmd.Stdout = &sizeOut
		var size int64
		if err := sizeCmd.Run(); err == nil {
			sizeStr := strings.TrimSpace(sizeOut.String())
			size, _ = strconv.ParseInt(sizeStr, 10, 64)
		}

		tableStats := TableStats{
			Name:     tableName,
			RowCount: rowCount,
			Size:     size,
		}

		stats.Tables = append(stats.Tables, tableStats)
		stats.TotalRows += rowCount
		stats.Size += size
	}

	return stats, nil
}

func analyzeMongoDB(database string, requiresAuth bool, password string) (*DatabaseStats, error) {
	stats := &DatabaseStats{
		Tables: []TableStats{},
	}

	// MongoDB uses collections instead of tables
	// Use mongosh (modern) or fallback to mongo (legacy)
	mongoCmd := "mongosh"
	if _, err := exec.LookPath("mongosh"); err != nil {
		mongoCmd = "mongo" // Fallback to legacy mongo shell
	}

	var cmd *exec.Cmd
	// Try connecting directly to the database first, then run the command
	// Format: mongosh database_name --quiet --eval "db.getCollectionNames()"
	// This is more reliable than using "use database" in the eval script
	if requiresAuth {
		cmd = exec.Command(mongoCmd, database, "--quiet", "--eval", "db.getCollectionNames()", "--username", "admin", "--password", password, "--authenticationDatabase", "admin")
	} else {
		cmd = exec.Command(mongoCmd, database, "--quiet", "--eval", "db.getCollectionNames()")
	}

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errMsg := stderr.String()
		if errMsg == "" {
			errMsg = err.Error()
		}
		// If database doesn't exist or is empty, return empty stats instead of error
		if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "doesn't exist") {
			return stats, nil // Return empty stats for non-existent database
		}
		return nil, fmt.Errorf("failed to list collections: %w\n%s", err, errMsg)
	}

	// Parse collection names from output (format: [ "collection1", "collection2" ])
	output := strings.TrimSpace(out.String())
	
	// First, try to find the array in the output (might be on multiple lines)
	// Look for pattern: [ "collection1", "collection2" ]
	fullOutput := output
	
	// Remove connection messages and warnings from mongosh output
	lines := strings.Split(fullOutput, "\n")
	cleanLines := []string{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip connection messages, warnings, etc.
		if line != "" && 
		   !strings.Contains(line, "Connecting to") &&
		   !strings.Contains(line, "Using MongoDB") &&
		   !strings.Contains(line, "Using Mongosh") &&
		   !strings.Contains(line, "Current Mongosh Log") &&
		   !strings.Contains(line, "For mongosh info") &&
		   !strings.HasPrefix(line, "---") &&
		   !strings.Contains(line, "Access control") &&
		   !strings.Contains(line, "filesystem") &&
		   !strings.Contains(line, "switched to db") &&
		   !strings.Contains(line, "MongoDB") &&
		   !strings.Contains(line, "Mongosh") {
			cleanLines = append(cleanLines, line)
		}
	}
	output = strings.Join(cleanLines, " ")
	
	// Try to extract array - look for [ ... ] pattern
	start := strings.Index(output, "[")
	end := strings.LastIndex(output, "]")
	if start >= 0 && end > start {
		output = output[start+1 : end]
	} else {
		// If no brackets, the output might just be collection names separated by commas
		// Or it might be a single collection name
		output = strings.TrimSpace(output)
	}
	
	// Clean up the output
	output = strings.TrimSpace(output)
	
	collectionNames := []string{}
	if output != "" {
		// Split by comma and clean each part
		parts := strings.Split(output, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			// Remove quotes (both single and double)
			part = strings.Trim(part, `"`)
			part = strings.Trim(part, `'`)
			// Remove any remaining brackets or whitespace
			part = strings.Trim(part, "[]{}")
			part = strings.TrimSpace(part)
			if part != "" {
				collectionNames = append(collectionNames, part)
			}
		}
	}
	
	// If we still don't have collections, try a different approach
	// Sometimes mongosh outputs just the collection name without brackets
	if len(collectionNames) == 0 && output != "" {
		// Try treating the whole output as a single collection name
		singleName := strings.TrimSpace(output)
		singleName = strings.Trim(singleName, `"'[]{}`)
		if singleName != "" && 
		   !strings.Contains(singleName, "Error") &&
		   !strings.Contains(singleName, "error") &&
		   !strings.Contains(singleName, "Connecting") {
			collectionNames = append(collectionNames, singleName)
		}
	}

	stats.TableCount = len(collectionNames)

	// Get document count for each collection
	// Reuse mongoCmd from above (already declared)

	for _, collectionName := range collectionNames {
		var countCmd *exec.Cmd
		// Connect directly to the database, then run countDocuments
		countEvalScript := fmt.Sprintf("db.%s.countDocuments()", collectionName)
		if requiresAuth {
			countCmd = exec.Command(mongoCmd, database, "--quiet", "--eval", countEvalScript, "--username", "admin", "--password", password, "--authenticationDatabase", "admin")
		} else {
			countCmd = exec.Command(mongoCmd, database, "--quiet", "--eval", countEvalScript)
		}

		var countOut bytes.Buffer
		var countErr bytes.Buffer
		countCmd.Stdout = &countOut
		countCmd.Stderr = &countErr
		rowCount := int64(0)
		if err := countCmd.Run(); err == nil {
			countStr := strings.TrimSpace(countOut.String())
			
			// Filter out mongosh connection messages and warnings
			lines := strings.Split(countStr, "\n")
			cleanCountLines := []string{}
			for _, line := range lines {
				line = strings.TrimSpace(line)
				// Skip connection messages, warnings, etc.
				if line != "" && 
				   !strings.Contains(line, "Connecting to") &&
				   !strings.Contains(line, "Using MongoDB") &&
				   !strings.Contains(line, "Using Mongosh") &&
				   !strings.Contains(line, "Current Mongosh Log") &&
				   !strings.Contains(line, "For mongosh info") &&
				   !strings.HasPrefix(line, "---") &&
				   !strings.Contains(line, "Access control") &&
				   !strings.Contains(line, "filesystem") &&
				   !strings.Contains(line, "switched to db") &&
				   !strings.Contains(line, "Error") &&
				   !strings.Contains(strings.ToLower(line), "error") {
					cleanCountLines = append(cleanCountLines, line)
				}
			}
			countStr = strings.Join(cleanCountLines, "\n")
			countStr = strings.TrimSpace(countStr)
			
			// Parse the count - should be just a number
			if countStr != "" {
				// Try to extract number from string (in case there's extra output)
				parsed, parseErr := strconv.ParseInt(countStr, 10, 64)
				if parseErr == nil {
					rowCount = parsed
				} else {
					// Try to find number in the string (handle cases like "2" or "Result: 2")
					for _, word := range strings.Fields(countStr) {
						// Remove any non-numeric characters
						word = strings.Trim(word, "()[]{},.;:!?")
						if parsed, parseErr := strconv.ParseInt(word, 10, 64); parseErr == nil {
							rowCount = parsed
							break
						}
					}
				}
			}
		} else {
			// Log error but continue - might be a permission issue or collection doesn't exist
			errMsg := countErr.String()
			if errMsg == "" {
				errMsg = err.Error()
			}
			// Don't fail completely, just log and continue with 0 count
		}

		tableStats := TableStats{
			Name:     collectionName,
			RowCount: rowCount,
		}

		stats.Tables = append(stats.Tables, tableStats)
		stats.TotalRows += rowCount
	}

	return stats, nil
}

func analyzeSQLite(database string) (*DatabaseStats, error) {
	stats := &DatabaseStats{
		Tables: []TableStats{},
	}

	// Get list of tables
	cmd := exec.Command("sqlite3", database, ".tables")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to list tables: %w", err)
	}

	tableNames := []string{}
	for _, line := range strings.Split(out.String(), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			// Tables might be space-separated on one line
			for _, name := range strings.Fields(line) {
				tableNames = append(tableNames, name)
			}
		}
	}

	stats.TableCount = len(tableNames)

	// Get row count for each table
	for _, tableName := range tableNames {
		countCmd := exec.Command("sqlite3", database, fmt.Sprintf("SELECT COUNT(*) FROM %s;", tableName))
		var countOut bytes.Buffer
		countCmd.Stdout = &countOut
		rowCount := int64(0)
		if err := countCmd.Run(); err == nil {
			countStr := strings.TrimSpace(countOut.String())
			rowCount, _ = strconv.ParseInt(countStr, 10, 64)
		}

		tableStats := TableStats{
			Name:     tableName,
			RowCount: rowCount,
		}

		stats.Tables = append(stats.Tables, tableStats)
		stats.TotalRows += rowCount
	}

	// Get database file size
	if info, err := os.Stat(database); err == nil {
		stats.Size = info.Size()
	}

	return stats, nil
}

func analyzeRedis(database string, requiresAuth bool, password string) (*DatabaseStats, error) {
	stats := &DatabaseStats{
		Tables: []TableStats{},
	}

	// Redis uses keys instead of tables
	// Get key count using DBSIZE
	var cmd *exec.Cmd
	if requiresAuth {
		cmd = exec.Command("redis-cli", "-a", password, "DBSIZE")
	} else {
		cmd = exec.Command("redis-cli", "DBSIZE")
	}

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	
	keyCount := int64(0)
	if err := cmd.Run(); err == nil {
		output := strings.TrimSpace(out.String())
		// DBSIZE returns just a number
		if parsed, parseErr := strconv.ParseInt(output, 10, 64); parseErr == nil {
			keyCount = parsed
		}
	}

	// Get all keys to show as "tables" (grouped by key pattern)
	var keysCmd *exec.Cmd
	if requiresAuth {
		keysCmd = exec.Command("redis-cli", "-a", password, "KEYS", "*")
	} else {
		keysCmd = exec.Command("redis-cli", "KEYS", "*")
	}

	var keysOut bytes.Buffer
	keysCmd.Stdout = &keysOut
	keysCmd.Run() // Ignore errors

	// Parse keys and group by pattern
	keysOutput := strings.TrimSpace(keysOut.String())
	keyPatterns := make(map[string]int64)
	
	if keysOutput != "" && keysOutput != "(empty array)" {
		// Remove brackets if present
		keysOutput = strings.TrimPrefix(keysOutput, "[")
		keysOutput = strings.TrimSuffix(keysOutput, "]")
		keysOutput = strings.TrimSpace(keysOutput)
		
		lines := strings.Split(keysOutput, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			// Remove quotes
			line = strings.Trim(line, `"`)
			line = strings.Trim(line, `'`)
			if line != "" {
				// Extract pattern (e.g., "user:1" -> "user:*", "user:1:name" -> "user:*:*")
				parts := strings.Split(line, ":")
				if len(parts) > 0 {
					pattern := parts[0] + ":*"
					keyPatterns[pattern]++
				} else {
					keyPatterns[line]++
				}
			}
		}
	}

	// Create table stats for each pattern
	for pattern, count := range keyPatterns {
		tableStats := TableStats{
			Name:     pattern,
			RowCount: count,
		}
		stats.Tables = append(stats.Tables, tableStats)
		stats.TotalRows += count
	}

	// If no patterns found but we have keys, create a generic entry
	if len(stats.Tables) == 0 && keyCount > 0 {
		tableStats := TableStats{
			Name:     "keys",
			RowCount: keyCount,
		}
		stats.Tables = append(stats.Tables, tableStats)
		stats.TotalRows = keyCount
	}

	stats.TableCount = len(stats.Tables)
	
	// Get database size (approximate)
	var sizeCmd *exec.Cmd
	if requiresAuth {
		sizeCmd = exec.Command("redis-cli", "-a", password, "INFO", "memory")
	} else {
		sizeCmd = exec.Command("redis-cli", "INFO", "memory")
	}

	var sizeOut bytes.Buffer
	sizeCmd.Stdout = &sizeOut
	if err := sizeCmd.Run(); err == nil {
		// Parse used_memory from INFO output
		output := sizeOut.String()
		for _, line := range strings.Split(output, "\n") {
			if strings.HasPrefix(line, "used_memory:") {
				parts := strings.Split(line, ":")
				if len(parts) >= 2 {
					if size, parseErr := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64); parseErr == nil {
						stats.Size = size
						break
					}
				}
			}
		}
	}

	return stats, nil
}
