package execute

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"mirrorvault/internal/backup/credentials"
	"mirrorvault/internal/config"
	"mirrorvault/internal/restore/log"
	restoreplan "mirrorvault/internal/restore/plan"
	"mirrorvault/internal/restore/validate"
)

func restoreMongoDB(
	restorePlan *restoreplan.RestorePlan,
	dumpPath string,
	dumpInfo *validate.DumpInfo,
	authCtx *credentials.AuthContext,
	logger *log.Logger,
	onProgress func(string, float64, string, error),
) error {
	logger.Info("Starting MongoDB restore")

	password := ""
	if restorePlan.RequiresAuth {
		pwd, ok := authCtx.Get("MongoDB")
		if !ok {
			return fmt.Errorf("missing MongoDB credentials")
		}
		password = pwd
	}

	user := config.MongoUser()
	authDB := config.MongoAuthDB()

	// Step 1: Drop existing database
	onProgress("Preparing database", 0.5, "Dropping existing database...", nil)
	logger.Info("Dropping existing database")

	// Use mongosh (modern) or fallback to mongo (legacy)
	mongoCmd := "mongosh"
	if _, err := exec.LookPath("mongosh"); err != nil {
		mongoCmd = "mongo" // Fallback to legacy mongo shell
	}

	var dropCmd *exec.Cmd
	if restorePlan.RequiresAuth {
		dropCmd = exec.Command(mongoCmd, restorePlan.Database, "--quiet", "--eval", "db.dropDatabase()", "--username", user, "--password", password, "--authenticationDatabase", authDB)
	} else {
		dropCmd = exec.Command(mongoCmd, restorePlan.Database, "--quiet", "--eval", "db.dropDatabase()")
	}
	dropCmd.Stderr = os.Stderr
	dropCmd.Run() // Ignore errors if database doesn't exist

	// Step 2: Restore from dump
	onProgress("Restoring data", 0.6, "Importing data from dump...", nil)
	logger.Info("Importing data from dump")

	// MongoDB dumps can be:
	// 1. Directory format: dump_dir/database_name/collections.bson (standard mongodump)
	// 2. Archive format: single file (mongodump --archive or --archive --gzip)
	info, err := os.Stat(dumpPath)
	if err != nil {
		return fmt.Errorf("failed to stat dump path: %w", err)
	}
	
	// Check if it's an archive format (single file, possibly compressed)
	if !info.IsDir() {
		// Archive format - mongorestore can handle this directly
		logger.Info("Detected MongoDB archive format dump")
		restorePath := dumpPath
		cleanup := func() {}
		if dumpInfo.Compressed && dumpInfo.Compression != "gz" {
			var err error
			restorePath, cleanup, err = writeDecompressedTempFileMongo(dumpPath, dumpInfo, ".archive")
			if err != nil {
				return err
			}
		}
		defer cleanup()

		var restoreCmd *exec.Cmd
		if restorePlan.RequiresAuth {
			restoreCmd = exec.Command(
				"mongorestore",
				"--archive",
				"--db", restorePlan.Database,
				"--username", user,
				"--password", password,
				"--authenticationDatabase", authDB,
			)
		} else {
			restoreCmd = exec.Command(
				"mongorestore",
				"--archive",
				"--db", restorePlan.Database,
			)
		}
		
		// Handle compression for archive
		if dumpInfo.Compressed && dumpInfo.Compression == "gz" {
			restoreCmd.Args = append(restoreCmd.Args, "--gzip")
		}
		
		// Read from file
		file, err := os.Open(restorePath)
		if err != nil {
			return fmt.Errorf("failed to open archive file: %w", err)
		}
		defer file.Close()
		
		restoreCmd.Stdin = file
		var stderr bytes.Buffer
		var stdout bytes.Buffer
		restoreCmd.Stderr = &stderr
		restoreCmd.Stdout = &stdout
		restoreCmd.Stderr = io.MultiWriter(&stderr, os.Stderr)
		restoreCmd.Stdout = io.MultiWriter(&stdout, os.Stdout)
		
		if err := restoreCmd.Run(); err != nil {
			errMsg := stderr.String()
			if errMsg == "" {
				errMsg = err.Error()
			}
			return fmt.Errorf("failed to restore from archive: %v\n%s", err, errMsg)
		}
		
		output := stdout.String() + stderr.String()
		if strings.Contains(output, "done restoring") || strings.Contains(output, "restored successfully") {
			logger.Info("MongoDB restore from archive completed successfully")
		}
		return nil
	}

	// Check if dumpPath contains the database directory directly
	// or if it's a multi-database dump with database subdirectories
	dbDir := filepath.Join(dumpPath, restorePlan.Database)
	dbDirExists := false
	if info, err := os.Stat(dbDir); err == nil && info.IsDir() {
		dbDirExists = true
		logger.Info(fmt.Sprintf("Found database directory: %s", dbDir))
	}

	// Determine restore directory
	// If database subdirectory exists, use it directly and explicitly specify --db
	// This ensures mongorestore restores to the correct database name
	var restoreCmd *exec.Cmd
	if dbDirExists {
		// Database directory exists: mongodump created dump_dir/dbname/collections.bson
		// Use --dir with database directory and explicitly specify --db to ensure correct target
		logger.Info(fmt.Sprintf("Using database directory: %s", dbDir))
		if restorePlan.RequiresAuth {
			restoreCmd = exec.Command(
				"mongorestore",
				"--dir", dbDir,
				"--db", restorePlan.Database,
				"--username", user,
				"--password", password,
				"--authenticationDatabase", authDB,
			)
		} else {
			restoreCmd = exec.Command(
				"mongorestore",
				"--dir", dbDir,
				"--db", restorePlan.Database,
			)
		}
	} else {
		// Database directory doesn't exist, try using parent with --db flag
		// This handles cases where dump structure is different
		logger.Info(fmt.Sprintf("Database directory not found, using parent directory: %s with --db %s", dumpPath, restorePlan.Database))
		if restorePlan.RequiresAuth {
			restoreCmd = exec.Command(
				"mongorestore",
				"--db", restorePlan.Database,
				"--dir", dumpPath,
				"--username", user,
				"--password", password,
				"--authenticationDatabase", authDB,
			)
		} else {
			restoreCmd = exec.Command(
				"mongorestore",
				"--db", restorePlan.Database,
				"--dir", dumpPath,
			)
		}
	}

	var stderr bytes.Buffer
	var stdout bytes.Buffer
	restoreCmd.Stderr = &stderr
	restoreCmd.Stdout = &stdout
	// Also show output in real-time
	restoreCmd.Stderr = io.MultiWriter(&stderr, os.Stderr)
	restoreCmd.Stdout = io.MultiWriter(&stdout, os.Stdout)

	logger.Info(fmt.Sprintf("Running mongorestore command: %v", restoreCmd.Args))
	
	if err := restoreCmd.Run(); err != nil {
		errMsg := stderr.String()
		if errMsg == "" {
			errMsg = err.Error()
		}
		logger.Error(fmt.Sprintf("mongorestore command failed: %v\nStderr: %s\nStdout: %s", err, errMsg, stdout.String()))
		return fmt.Errorf("failed to restore database: %v\n%s", err, errMsg)
	}

	// Check output for success indicators
	output := stdout.String() + stderr.String()
	logger.Info(fmt.Sprintf("mongorestore output:\nStdout: %s\nStderr: %s", stdout.String(), stderr.String()))
	
	if strings.Contains(output, "done restoring") || 
	   strings.Contains(output, "restored successfully") ||
	   strings.Contains(output, "finished restoring") ||
	   strings.Contains(output, "restoring") {
		logger.Info("MongoDB restore completed successfully")
	} else {
		// Even if no clear success message, check if any collections were restored
		if strings.Contains(output, "documents") || strings.Contains(output, "collections") {
			logger.Info("MongoDB restore appears to have completed (found collection/document references in output)")
		} else {
			logger.Warning("MongoDB restore completed but output doesn't show clear success message or data restoration")
			logger.Warning(fmt.Sprintf("Full output: %s", output))
		}
	}

	logger.Info("MongoDB restore completed successfully")
	return nil
}

func writeDecompressedTempFileMongo(dumpPath string, dumpInfo *validate.DumpInfo, ext string) (string, func(), error) {
	reader, closeReader, err := validate.OpenDecompressedReader(dumpPath, dumpInfo)
	if err != nil {
		return "", func() {}, err
	}

	tmpFile, err := os.CreateTemp("", "mirrorvault_restore_*"+ext)
	if err != nil {
		_ = closeReader()
		return "", func() {}, err
	}

	if _, err := io.Copy(tmpFile, reader); err != nil {
		_ = tmpFile.Close()
		_ = closeReader()
		_ = os.Remove(tmpFile.Name())
		return "", func() {}, err
	}
	_ = tmpFile.Close()
	_ = closeReader()

	cleanup := func() {
		_ = os.Remove(tmpFile.Name())
	}
	return tmpFile.Name(), cleanup, nil
}

func rollbackMongoDB(
	restorePlan *restoreplan.RestorePlan,
	backupPath string,
	authCtx *credentials.AuthContext,
	logger *log.Logger,
) error {
	logger.Info("Starting MongoDB rollback")

	password := ""
	if restorePlan.RequiresAuth {
		pwd, ok := authCtx.Get("MongoDB")
		if !ok {
			return fmt.Errorf("missing MongoDB credentials")
		}
		password = pwd
	}

	user := config.MongoUser()
	authDB := config.MongoAuthDB()

	// Drop database
	// Use mongosh (modern) or fallback to mongo (legacy)
	mongoCmd := "mongosh"
	if _, err := exec.LookPath("mongosh"); err != nil {
		mongoCmd = "mongo" // Fallback to legacy mongo shell
	}

	var dropCmd *exec.Cmd
	if restorePlan.RequiresAuth {
		dropCmd = exec.Command(mongoCmd, restorePlan.Database, "--quiet", "--eval", "db.dropDatabase()", "--username", user, "--password", password, "--authenticationDatabase", authDB)
	} else {
		dropCmd = exec.Command(mongoCmd, restorePlan.Database, "--quiet", "--eval", "db.dropDatabase()")
	}
	dropCmd.Stderr = os.Stderr
	dropCmd.Run()

	// Restore from backup
	var restoreCmd *exec.Cmd
	if restorePlan.RequiresAuth {
		restoreCmd = exec.Command(
			"mongorestore",
			"--db", restorePlan.Database,
			"--dir", backupPath,
			"--username", user,
			"--password", password,
			"--authenticationDatabase", authDB,
		)
	} else {
		restoreCmd = exec.Command(
			"mongorestore",
			"--db", restorePlan.Database,
			"--dir", backupPath,
		)
	}

	restoreCmd.Stderr = os.Stderr
	restoreCmd.Stdout = os.Stdout

	if err := restoreCmd.Run(); err != nil {
		return fmt.Errorf("failed to restore from backup: %w", err)
	}

	logger.Info("MongoDB rollback completed successfully")
	return nil
}
