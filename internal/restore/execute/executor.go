package execute

import (
	"fmt"
	"os"
	"path/filepath"

	"mirrorvault/internal/backup/credentials"
	"mirrorvault/internal/restore/analyze"
	"mirrorvault/internal/restore/log"
	restoreplan "mirrorvault/internal/restore/plan"
	"mirrorvault/internal/restore/validate"
)

type ProgressFunc func(step string, progress float64, message string, err error)

type RestoreResult struct {
	Success          bool
	PreRestoreBackup string
	PostRestoreStats *analyze.DatabaseStats
	Error            error
	RolledBack       bool
}

func Run(
	restorePlan *restoreplan.RestorePlan,
	authCtx *credentials.AuthContext,
	onProgress ProgressFunc,
) (*RestoreResult, error) {
	// Create logger
	logger, err := log.NewLogger(fmt.Sprintf("%s_%s", restorePlan.Engine, restorePlan.Database))
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}
	defer logger.Close()

	logger.Info(fmt.Sprintf("Starting restore operation for %s/%s", restorePlan.Engine, restorePlan.Database))
	logger.Info(fmt.Sprintf("Dump path: %s", restorePlan.DumpPath))

	// Step 1: Validate dump
	onProgress("Validating dump", 0.1, "Analyzing dump file...", nil)
	logger.Info("Step 1: Validating dump file")
	dumpInfo, err := validate.ValidateDump(restorePlan.DumpPath)
	if err != nil {
		logger.Error(fmt.Sprintf("Dump validation failed: %v", err))
		onProgress("Validation failed", 0.0, "", err)
		return &RestoreResult{Success: false, Error: err}, err
	}
	logger.Info(fmt.Sprintf("Dump format: %s, Compressed: %v, Multi-DB: %v", dumpInfo.Format, dumpInfo.Compressed, dumpInfo.IsMultiDB))

	// Step 1.5: Validate format compatibility with selected engine
	onProgress("Validating dump", 0.12, "Checking format compatibility...", nil)
	logger.Info("Step 1.5: Validating dump format compatibility with selected engine")
	if err := validate.ValidateFormatCompatibility(dumpInfo, restorePlan.Engine); err != nil {
		logger.Error(fmt.Sprintf("Format compatibility check failed: %v", err))
		onProgress("Format mismatch", 0.0, "", err)
		return &RestoreResult{Success: false, Error: err}, err
	}
	logger.Info(fmt.Sprintf("Dump format '%s' is compatible with engine '%s'", dumpInfo.Format, restorePlan.Engine))

	// Step 2: Extract target database from dump if needed
	// Note: MongoDB directory dumps don't need extraction - we use the directory path directly
	actualDumpPath := restorePlan.DumpPath
	if dumpInfo.IsMultiDB && dumpInfo.Format != "mongodb" {
		// Only extract for SQL dumps (MySQL/PostgreSQL)
		// MongoDB directory dumps are handled directly by the restore function
		onProgress("Extracting database", 0.15, "Extracting target database from multi-DB dump...", nil)
		logger.Info("Step 2: Extracting target database from multi-DB dump")
		extractedPath, err := validate.ExtractDatabaseFromDump(restorePlan.DumpPath, restorePlan.Database, dumpInfo)
		if err != nil {
			logger.Error(fmt.Sprintf("Failed to extract database: %v", err))
			onProgress("Extraction failed", 0.0, "", err)
			return &RestoreResult{Success: false, Error: err}, err
		}
		actualDumpPath = extractedPath
		logger.Info(fmt.Sprintf("Extracted database dump to: %s", extractedPath))
	} else if dumpInfo.IsMultiDB && dumpInfo.Format == "mongodb" {
		// For MongoDB multi-database dumps, verify the database directory exists
		logger.Info("Step 2: Verifying target database exists in MongoDB dump")
		dbDir := filepath.Join(restorePlan.DumpPath, restorePlan.Database)
		if info, err := os.Stat(dbDir); err != nil || !info.IsDir() {
			err := fmt.Errorf("target database '%s' not found in dump directory '%s'", restorePlan.Database, restorePlan.DumpPath)
			logger.Error(fmt.Sprintf("Database directory not found: %v", err))
			onProgress("Validation failed", 0.0, "", err)
			return &RestoreResult{Success: false, Error: err}, err
		}
		logger.Info(fmt.Sprintf("Found database directory: %s", dbDir))
		// actualDumpPath remains the parent directory - MongoDB restore function will handle it
	}

	// Step 2.5: Validate dump contains data
	dumpTables, err := validate.ExtractTablesFromDump(actualDumpPath, dumpInfo.Compressed, dumpInfo.Compression)
	if err != nil {
		logger.Warning(fmt.Sprintf("Could not analyze dump tables: %v", err))
	} else if len(dumpTables) == 0 {
		logger.Warning("Dump appears to contain no tables - this may indicate an issue with the dump file")
		onProgress("Warning", 0.2, "Dump file appears empty or could not be analyzed", nil)
	} else {
		logger.Info(fmt.Sprintf("Dump contains %d tables: %v", len(dumpTables), dumpTables))
	}

	// Step 3: Analyze current database
	onProgress("Analyzing database", 0.2, "Collecting current database statistics...", nil)
	logger.Info("Step 3: Analyzing current database")
	password := ""
	if restorePlan.RequiresAuth {
		pwd, ok := authCtx.Get(restorePlan.Engine)
		if !ok {
			err := fmt.Errorf("missing credentials for %s", restorePlan.Engine)
			logger.Error(err.Error())
			onProgress("Authentication failed", 0.0, "", err)
			return &RestoreResult{Success: false, Error: err}, err
		}
		password = pwd
	}

	preRestoreStats, err := analyze.AnalyzeDatabase(restorePlan.Engine, restorePlan.Database, restorePlan.RequiresAuth, password)
	if err != nil {
		logger.Warning(fmt.Sprintf("Failed to analyze current database (may not exist): %v", err))
		// Continue anyway - database might not exist yet
		preRestoreStats = &analyze.DatabaseStats{}
	} else {
		logger.Info(fmt.Sprintf("Current database: %d tables, %d total rows", preRestoreStats.TableCount, preRestoreStats.TotalRows))
	}

	// Step 4: Create pre-restore backup
	onProgress("Creating backup", 0.3, "Creating backup of current database...", nil)
	logger.Info("Step 4: Creating pre-restore backup")
	backupPath, err := createPreRestoreBackup(restorePlan, authCtx, logger)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to create pre-restore backup: %v", err))
		onProgress("Backup failed", 0.0, "", err)
		return &RestoreResult{Success: false, Error: err}, err
	}
	logger.Info(fmt.Sprintf("Pre-restore backup created: %s", backupPath))

	// Step 5: Restore database
	onProgress("Restoring database", 0.5, "Restoring database from dump...", nil)
	logger.Info("Step 5: Restoring database")
	err = restoreDatabase(restorePlan, actualDumpPath, dumpInfo, authCtx, logger, onProgress)
	if err != nil {
		logger.Error(fmt.Sprintf("Restore failed: %v", err))
		logger.Info("Initiating automatic rollback...")

		// Step 6: Rollback on error
		onProgress("Rolling back", 0.8, "Restoring from backup due to error...", nil)
		rollbackErr := rollbackFromBackup(restorePlan, backupPath, authCtx, logger)
		if rollbackErr != nil {
			logger.Error(fmt.Sprintf("Rollback failed: %v", rollbackErr))
			onProgress("Rollback failed", 0.0, "", rollbackErr)
			return &RestoreResult{
				Success:          false,
				PreRestoreBackup: backupPath,
				Error:            fmt.Errorf("restore failed: %v, rollback also failed: %v", err, rollbackErr),
				RolledBack:       false,
			}, rollbackErr
		}

		logger.Info("Rollback completed successfully")
		onProgress("Rollback complete", 1.0, "Database restored to previous state", nil)
		return &RestoreResult{
			Success:          false,
			PreRestoreBackup: backupPath,
			Error:            err,
			RolledBack:       true,
		}, err
	}

	// Step 7: Validate restore
	onProgress("Validating restore", 0.9, "Validating restored database...", nil)
	logger.Info("Step 7: Validating restored database")
	postRestoreStats, err := analyze.AnalyzeDatabase(restorePlan.Engine, restorePlan.Database, restorePlan.RequiresAuth, password)
	if err != nil {
		logger.Warning(fmt.Sprintf("Failed to analyze restored database: %v", err))
		// Continue anyway
		postRestoreStats = &analyze.DatabaseStats{}
	} else {
		logger.Info(fmt.Sprintf("Restored database: %d tables, %d total rows", postRestoreStats.TableCount, postRestoreStats.TotalRows))
		
		// Warn if restore resulted in empty database
		if postRestoreStats.TableCount == 0 && postRestoreStats.TotalRows == 0 {
			logger.Warning("WARNING: Restore completed but database is empty (0 tables, 0 rows)")
			logger.Warning("This may indicate the dump file was empty or contained no data")
			logger.Warning("Please verify the dump file contains valid data for the selected database")
		}
	}

	logger.Info("Restore operation completed successfully")
	onProgress("Complete", 1.0, "Restore completed successfully", nil)

	return &RestoreResult{
		Success:          true,
		PreRestoreBackup: backupPath,
		PostRestoreStats: postRestoreStats,
	}, nil
}
