package execute

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"mirrorvault/internal/backup/credentials"
	"mirrorvault/internal/backup/plan"
)

func runMongoDB(
	engine plan.EnginePlan,
	creds *credentials.AuthContext,
	onProgress ProgressFunc,
) error {

	baseDir := engine.OutputDir

	if err := ensureDir(baseDir); err != nil {
		return err
	}

	for _, db := range engine.Databases {
		// Generate filename with current date: dbname_YYYY-MM-DD
		currentDate := time.Now().Format("2006-01-02")
		// MongoDB backup creates a directory, so we'll name it dbname_YYYY-MM-DD
		backupDirName := fmt.Sprintf("%s_%s", db.Name, currentDate)
		outDir := filepath.Join(baseDir, backupDirName)

		// ▶ running
		onProgress(
			engine.Engine,
			db.Name,
			outDir,
			0,
			"running",
			nil,
		)

		var cmd *exec.Cmd

		if engine.RequiresAuth {
			pwd, ok := creds.Get("MongoDB")
			if !ok {
				err := fmt.Errorf("missing MongoDB credentials")
				onProgress(engine.Engine, db.Name, "", 0, "failed", err)
				return err
			}

			// mongodump with authentication
			// Use "admin" as default username (most common MongoDB setup)
			// Note: In production, username might need to be configurable
			cmd = exec.Command(
				"mongodump",
				"--db", db.Name,
				"--out", outDir,
				"--username", "admin",
				"--password", pwd,
				"--authenticationDatabase", "admin",
			)
		} else {
			// mongodump without authentication
			cmd = exec.Command(
				"mongodump",
				"--db", db.Name,
				"--out", outDir,
			)
		}

		var stderr bytes.Buffer
		cmd.Stderr = io.MultiWriter(&stderr, os.Stderr)

		err := cmd.Run()
		if err != nil {
			stderrStr := stderr.String()
			// Check if it's a connection or data corruption error
			stderrBytes := []byte(stderrStr)
			isDataError := bytes.Contains(stderrBytes, []byte("corrupt")) ||
			              bytes.Contains(stderrBytes, []byte("invalid")) ||
			              bytes.Contains(stderrBytes, []byte("connection")) ||
			              bytes.Contains(stderrBytes, []byte("timeout")) ||
			              bytes.Contains(stderrBytes, []byte("E11000")) // Duplicate key error
			
			// Try fallback with different flags
			if isDataError {
				// Retry with --numParallelCollections=1 and --gzip (more compatible)
				var cmd2 *exec.Cmd
				var stderr2 bytes.Buffer
				
				if engine.RequiresAuth {
					pwd, _ := creds.Get("MongoDB")
					cmd2 = exec.Command(
						"mongodump",
						"--db", db.Name,
						"--out", outDir,
						"--username", "admin",
						"--password", pwd,
						"--authenticationDatabase", "admin",
						"--numParallelCollections", "1", // Dump one collection at a time
					)
				} else {
					cmd2 = exec.Command(
						"mongodump",
						"--db", db.Name,
						"--out", outDir,
						"--numParallelCollections", "1",
					)
				}
				
				cmd2.Stderr = io.MultiWriter(&stderr2, os.Stderr)
				
				if err2 := cmd2.Run(); err2 != nil {
					// Both attempts failed - try dumping collections individually as last resort
					if err3 := tryDumpMongoCollectionsIndividually(engine, creds, db.Name, outDir, onProgress); err3 != nil {
						errMsg := fmt.Errorf("mongodump failed (tried multiple approaches):\n1. Standard: %v\n%s\n2. --numParallelCollections=1: %v\n%s\n3. Individual collections: %v", 
							err, stderrStr, err2, stderr2.String(), err3)
						onProgress(engine.Engine, db.Name, "", 0, "failed", errMsg)
						return errMsg
					}
					// Individual collection dump succeeded
				}
				// Fallback succeeded
			} else {
				// Not a data error, return original error
				errMsg := fmt.Errorf("mongodump failed: %v", err)
				if stderr.Len() > 0 {
					errMsg = fmt.Errorf("mongodump failed: %v\n%s", err, stderr.String())
				}
				onProgress(engine.Engine, db.Name, "", 0, "failed", errMsg)
				return errMsg
			}
		}

		// Calculate total size of backup directory
		var size int64
		err = filepath.Walk(outDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				size += info.Size()
			}
			return nil
		})

		if err != nil {
			// If size calculation fails, just report 0
			size = 0
		}

		// ✔ done
		onProgress(
			engine.Engine,
			db.Name,
			outDir,
			size,
			"done",
			nil,
		)
	}

	return nil
}

// tryDumpMongoCollectionsIndividually attempts to dump collections one by one when full database dump fails
func tryDumpMongoCollectionsIndividually(
	engine plan.EnginePlan,
	creds *credentials.AuthContext,
	dbName string,
	outDir string,
	onProgress ProgressFunc,
) error {
	// Get list of collections in the database
	var listCmd *exec.Cmd
	if engine.RequiresAuth {
		pwd, _ := creds.Get("MongoDB")
		listCmd = exec.Command(
			"mongo",
			dbName,
			"--quiet",
			"--eval", "db.getCollectionNames().join('\\n')",
			"--username", "admin",
			"--password", pwd,
			"--authenticationDatabase", "admin",
		)
	} else {
		listCmd = exec.Command(
			"mongo",
			dbName,
			"--quiet",
			"--eval", "db.getCollectionNames().join('\\n')",
		)
	}
	
	var collectionsOut bytes.Buffer
	listCmd.Stdout = &collectionsOut
	listCmd.Stderr = os.Stderr
	
	if err := listCmd.Run(); err != nil {
		return fmt.Errorf("failed to list collections: %v", err)
	}
	
	// Parse collection names
	collectionNames := []string{}
	for _, line := range strings.Split(collectionsOut.String(), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "MongoDB") {
			collectionNames = append(collectionNames, line)
		}
	}
	
	if len(collectionNames) == 0 {
		return fmt.Errorf("no collections found in database")
	}
	
	// Create output directory structure
	collectionDir := filepath.Join(outDir, dbName)
	if err := os.MkdirAll(collectionDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}
	
	// Try to dump each collection individually
	failedCollections := []string{}
	successCount := 0
	
	for _, collectionName := range collectionNames {
		var dumpCmd *exec.Cmd
		
		if engine.RequiresAuth {
			pwd, _ := creds.Get("MongoDB")
			dumpCmd = exec.Command(
				"mongodump",
				"--db", dbName,
				"--collection", collectionName,
				"--out", outDir,
				"--username", "admin",
				"--password", pwd,
				"--authenticationDatabase", "admin",
			)
		} else {
			dumpCmd = exec.Command(
				"mongodump",
				"--db", dbName,
				"--collection", collectionName,
				"--out", outDir,
			)
		}
		
		var collectionStderr bytes.Buffer
		dumpCmd.Stderr = io.MultiWriter(&collectionStderr, os.Stderr)
		
		if err := dumpCmd.Run(); err != nil {
			// This collection failed - log it and continue with others
			failedCollections = append(failedCollections, fmt.Sprintf("%s: %v", collectionName, err))
			continue
		}
		
		successCount++
	}
	
	if successCount == 0 {
		return fmt.Errorf("failed to dump any collections. Failed collections: %v", failedCollections)
	}
	
	if len(failedCollections) > 0 {
		// Some collections failed but we got some data
		onProgress(engine.Engine, dbName, outDir, 0, "done", 
			fmt.Errorf("partial backup: %d/%d collections dumped. Failed: %v", 
				successCount, len(collectionNames), failedCollections))
		return nil
	}
	
	return nil
}
