package execute

import (
	"fmt"

	"mirrorvault/internal/backup/credentials"
	"mirrorvault/internal/restore/log"
	restoreplan "mirrorvault/internal/restore/plan"
	"mirrorvault/internal/restore/validate"
)

func restoreDatabase(
	restorePlan *restoreplan.RestorePlan,
	dumpPath string,
	dumpInfo *validate.DumpInfo,
	authCtx *credentials.AuthContext,
	logger *log.Logger,
	onProgress func(string, float64, string, error),
) error {
	switch restorePlan.Engine {
	case "MySQL":
		return restoreMySQL(restorePlan, dumpPath, dumpInfo, authCtx, logger, onProgress)
	case "PostgreSQL":
		return restorePostgreSQL(restorePlan, dumpPath, dumpInfo, authCtx, logger, onProgress)
	case "MongoDB":
		return restoreMongoDB(restorePlan, dumpPath, dumpInfo, authCtx, logger, onProgress)
	case "SQLite":
		return restoreSQLite(restorePlan, dumpPath, dumpInfo, logger, onProgress)
	case "Redis":
		return restoreRedis(restorePlan, dumpPath, dumpInfo, authCtx, logger, onProgress)
	case "MSSQL":
		return restoreMSSQL(restorePlan, dumpPath, dumpInfo, authCtx, logger, onProgress)
	default:
		return fmt.Errorf("restore not implemented for engine: %s", restorePlan.Engine)
	}
}

func rollbackFromBackup(
	restorePlan *restoreplan.RestorePlan,
	backupPath string,
	authCtx *credentials.AuthContext,
	logger *log.Logger,
) error {
	switch restorePlan.Engine {
	case "MySQL":
		return rollbackMySQL(restorePlan, backupPath, authCtx, logger)
	case "PostgreSQL":
		return rollbackPostgreSQL(restorePlan, backupPath, authCtx, logger)
	case "MongoDB":
		return rollbackMongoDB(restorePlan, backupPath, authCtx, logger)
	case "SQLite":
		return rollbackSQLite(restorePlan, backupPath, logger)
	case "Redis":
		return rollbackRedis(restorePlan, backupPath, authCtx, logger)
	case "MSSQL":
		return rollbackMSSQL(restorePlan, backupPath, authCtx, logger)
	default:
		return fmt.Errorf("rollback not implemented for engine: %s", restorePlan.Engine)
	}
}
