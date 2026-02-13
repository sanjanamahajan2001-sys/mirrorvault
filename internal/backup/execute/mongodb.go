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
	"mirrorvault/internal/config"
)

func mongoBackupFormat() (string, bool) {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("MV_MONGO_BACKUP_FORMAT")))
	switch value {
	case "archive", "archive.gz", "archive_gz":
		return "archive", value == "archive.gz" || value == "archive_gz"
	default:
		return "directory", false
	}
}

func runMongoDB(
	engine plan.EnginePlan,
	creds *credentials.AuthContext,
	onProgress ProgressFunc,
) error {
	if err := requireCommand("mongodump"); err != nil {
		return err
	}
	if _, err := requireAnyCommand("mongosh", "mongo"); err != nil {
		return err
	}

	baseDir := engine.OutputDir
	user := config.MongoUser()
	authDB := config.MongoAuthDB()

	if err := ensureDir(baseDir); err != nil {
		return err
	}

	backupFormat, useGzip := mongoBackupFormat()

	if engine.AllDatabases {
		return runMongoDBAllDatabases(engine, creds, onProgress)
	}

	for _, db := range engine.Databases {
		// Generate filename with current date: dbname_YYYY-MM-DD
		currentDate := time.Now().Format("2006-01-02")
		backupDirName := fmt.Sprintf("%s_%s", db.Name, currentDate)
		outDir := filepath.Join(baseDir, backupDirName)
		outFile := ""
		if backupFormat == "archive" {
			ext := ".archive"
			if useGzip {
				ext = ".archive.gz"
			}
			outFile = filepath.Join(baseDir, fmt.Sprintf("%s_%s%s", db.Name, currentDate, ext))
		}

		// ▶ running
		progressPath := outDir
		if outFile != "" {
			progressPath = outFile
		}
		onProgress(
			engine.Engine,
			db.Name,
			progressPath,
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

			args := []string{"mongodump", "--db", db.Name, "--username", user, "--password", pwd, "--authenticationDatabase", authDB}
			if backupFormat == "archive" {
				args = append(args, "--archive="+outFile)
				if useGzip {
					args = append(args, "--gzip")
				}
			} else {
				args = append(args, "--out", outDir)
			}
			cmd = exec.Command(args[0], args[1:]...)
		} else {
			// mongodump without authentication
			args := []string{"mongodump", "--db", db.Name}
			if backupFormat == "archive" {
				args = append(args, "--archive="+outFile)
				if useGzip {
					args = append(args, "--gzip")
				}
			} else {
				args = append(args, "--out", outDir)
			}
			cmd = exec.Command(args[0], args[1:]...)
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
						"--username", user,
						"--password", pwd,
						"--authenticationDatabase", authDB,
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

		var size int64
		if backupFormat == "archive" {
			size, err = validateNonEmptyFile(outFile)
		} else {
			size, err = validateMongoDumpDir(outDir)
		}
		if err != nil {
			onProgress(engine.Engine, db.Name, "", 0, "failed", err)
			return err
		}
		if strictValidationEnabled() {
			if err := validateMongoDryRun(progressPath, db.Name, engine.RequiresAuth, creds); err != nil {
				onProgress(engine.Engine, db.Name, "", 0, "failed", err)
				return err
			}
		}

		if backupFormat == "directory" {
			compressedPath, compressedSize, err := applyBackupCompression(outDir, true)
			if err != nil {
				onProgress(engine.Engine, db.Name, "", 0, "failed", err)
				return err
			}
			progressPath = compressedPath
			size = compressedSize
		}

		// ✔ done
		onProgress(
			engine.Engine,
			db.Name,
			progressPath,
			size,
			"done",
			nil,
		)
	}

	return nil
}

func runMongoDBAllDatabases(
	engine plan.EnginePlan,
	creds *credentials.AuthContext,
	onProgress ProgressFunc,
) error {
	baseDir := engine.OutputDir
	user := config.MongoUser()
	authDB := config.MongoAuthDB()

	currentDate := time.Now().Format("2006-01-02")
	prefix := strings.ToLower(engine.Engine)
	dumpDir := filepath.Join(baseDir, fmt.Sprintf("%s_all_databases_%s", prefix, currentDate))
	outFile := dumpDir + ".tar"
	progressName := "All databases"

	onProgress(
		engine.Engine,
		progressName,
		outFile,
		0,
		"running",
		nil,
	)

	if len(engine.Databases) == 0 {
		err := fmt.Errorf("no databases available for MongoDB backup")
		onProgress(engine.Engine, progressName, "", 0, "failed", err)
		return err
	}

	_ = os.RemoveAll(dumpDir)
	if err := os.MkdirAll(dumpDir, 0755); err != nil {
		onProgress(engine.Engine, progressName, "", 0, "failed", err)
		return err
	}

	for _, db := range engine.Databases {
		var cmd *exec.Cmd
		if engine.RequiresAuth {
			pwd, ok := creds.Get("MongoDB")
			if !ok {
				err := fmt.Errorf("missing MongoDB credentials")
				onProgress(engine.Engine, progressName, "", 0, "failed", err)
				return err
			}
			args := []string{"mongodump", "--db", db.Name, "--out", dumpDir, "--username", user, "--password", pwd, "--authenticationDatabase", authDB}
			cmd = exec.Command(args[0], args[1:]...)
		} else {
			args := []string{"mongodump", "--db", db.Name, "--out", dumpDir}
			cmd = exec.Command(args[0], args[1:]...)
		}

		var stderr bytes.Buffer
		cmd.Stderr = io.MultiWriter(&stderr, os.Stderr)

		if err := cmd.Run(); err != nil {
			stderrStr := stderr.String()
			errMsg := fmt.Errorf("mongodump failed for %s: %v", db.Name, err)
			if stderrStr != "" {
				errMsg = fmt.Errorf("mongodump failed for %s: %v\n%s", db.Name, err, stderrStr)
			}
			onProgress(engine.Engine, progressName, "", 0, "failed", errMsg)
			return errMsg
		}
	}

	if _, err := validateMongoDumpDir(dumpDir); err != nil {
		onProgress(engine.Engine, progressName, "", 0, "failed", err)
		return err
	}
	if strictValidationEnabled() {
		if err := validateMongoDryRun(dumpDir, "", engine.RequiresAuth, creds); err != nil {
			onProgress(engine.Engine, progressName, "", 0, "failed", err)
			return err
		}
	}

	tarPath, err := tarDir(dumpDir)
	if err != nil {
		onProgress(engine.Engine, progressName, "", 0, "failed", err)
		return err
	}
	_ = os.RemoveAll(dumpDir)

	outFile = tarPath

	size, err := validateNonEmptyFile(outFile)
	if err != nil {
		onProgress(engine.Engine, progressName, "", 0, "failed", err)
		return err
	}

	compressedPath, compressedSize, err := applyBackupCompression(outFile, false)
	if err != nil {
		onProgress(engine.Engine, progressName, "", 0, "failed", err)
		return err
	}
	outFile = compressedPath
	size = compressedSize

	onProgress(
		engine.Engine,
		progressName,
		outFile,
		size,
		"done",
		nil,
	)

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
	user := config.MongoUser()
	authDB := config.MongoAuthDB()
	mongoCmd, err := requireAnyCommand("mongosh", "mongo")
	if err != nil {
		return err
	}

	// Get list of collections in the database
	var listCmd *exec.Cmd
	if engine.RequiresAuth {
		pwd, _ := creds.Get("MongoDB")
		listCmd = exec.Command(
			mongoCmd,
			dbName,
			"--quiet",
			"--eval", "db.getCollectionNames().join('\\n')",
			"--username", user,
			"--password", pwd,
			"--authenticationDatabase", authDB,
		)
	} else {
		listCmd = exec.Command(
			mongoCmd,
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
				"--username", user,
				"--password", pwd,
				"--authenticationDatabase", authDB,
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

func validateMongoDryRun(dumpPath string, dbName string, requiresAuth bool, creds *credentials.AuthContext) error {
	if err := requireCommand("mongorestore"); err != nil {
		return err
	}

	helpOutput, err := exec.Command("mongorestore", "--help").CombinedOutput()
	if err != nil {
		return nil
	}
	if !strings.Contains(string(helpOutput), "--dryRun") {
		return nil
	}

	args := []string{"mongorestore", "--dryRun"}
	if dbName != "" {
		args = append(args, "--db", dbName)
	}
	if info, err := os.Stat(dumpPath); err == nil && info.IsDir() {
		args = append(args, "--dir", dumpPath)
	} else {
		args = append(args, "--archive")
		if strings.HasSuffix(strings.ToLower(dumpPath), ".gz") {
			args = append(args, "--gzip")
		}
	}
	if requiresAuth {
		pwd, ok := creds.Get("MongoDB")
		if !ok {
			return fmt.Errorf("missing MongoDB credentials")
		}
		user := config.MongoUser()
		authDB := config.MongoAuthDB()
		args = append(args, "--username", user, "--password", pwd, "--authenticationDatabase", authDB)
	}

	cmd := exec.Command(args[0], args[1:]...)
	return cmd.Run()
}
