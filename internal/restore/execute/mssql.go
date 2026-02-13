package execute

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"

	"mirrorvault/internal/backup/credentials"
	"mirrorvault/internal/config"
	"mirrorvault/internal/restore/log"
	restoreplan "mirrorvault/internal/restore/plan"
	"mirrorvault/internal/restore/validate"
)

func restoreMSSQL(
	restorePlan *restoreplan.RestorePlan,
	dumpPath string,
	dumpInfo *validate.DumpInfo,
	authCtx *credentials.AuthContext,
	logger *log.Logger,
	onProgress func(string, float64, string, error),
) error {
	logger.Info("Starting MSSQL restore")
	onProgress("Preparing database", 0.5, "Restoring SQL Server database...", nil)

	restorePath := dumpPath
	cleanup := func() {}
	if dumpInfo != nil && dumpInfo.Compressed {
		var err error
		restorePath, cleanup, err = writeDecompressedTempFileMSSQL(dumpPath, dumpInfo, ".bak")
		if err != nil {
			return err
		}
	}
	defer cleanup()

	if _, err := os.Stat(restorePath); err != nil {
		return fmt.Errorf("backup file not found: %w", err)
	}

	password := ""
	if restorePlan.RequiresAuth {
		pwd, ok := authCtx.Get("MSSQL")
		if !ok {
			return fmt.Errorf("missing MSSQL credentials")
		}
		password = pwd
	}

	server := config.MSSQLServer()
	user := config.MSSQLUser()
	restoreQuery := fmt.Sprintf(
		"ALTER DATABASE [%s] SET SINGLE_USER WITH ROLLBACK IMMEDIATE; "+
			"RESTORE DATABASE [%s] FROM DISK = N'%s' WITH REPLACE; "+
			"ALTER DATABASE [%s] SET MULTI_USER;",
		restorePlan.Database,
		restorePlan.Database,
		restorePath,
		restorePlan.Database,
	)

	var cmd *exec.Cmd
	if restorePlan.RequiresAuth {
		cmd = exec.Command("sqlcmd", "-S", server, "-U", user, "-P", password, "-Q", restoreQuery, "-b")
	} else {
		cmd = exec.Command("sqlcmd", "-S", server, "-E", "-Q", restoreQuery, "-b")
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := stderr.String()
		if errMsg == "" {
			errMsg = err.Error()
		}
		return fmt.Errorf("failed to restore MSSQL database: %v\n%s", err, errMsg)
	}

	logger.Info("MSSQL restore completed successfully")
	return nil
}

func writeDecompressedTempFileMSSQL(dumpPath string, dumpInfo *validate.DumpInfo, ext string) (string, func(), error) {
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

func rollbackMSSQL(
	restorePlan *restoreplan.RestorePlan,
	backupPath string,
	authCtx *credentials.AuthContext,
	logger *log.Logger,
) error {
	logger.Info("Starting MSSQL rollback")

	if _, err := os.Stat(backupPath); err != nil {
		return fmt.Errorf("backup file not found: %w", err)
	}

	password := ""
	if restorePlan.RequiresAuth {
		pwd, ok := authCtx.Get("MSSQL")
		if !ok {
			return fmt.Errorf("missing MSSQL credentials")
		}
		password = pwd
	}

	server := config.MSSQLServer()
	user := config.MSSQLUser()
	restoreQuery := fmt.Sprintf(
		"ALTER DATABASE [%s] SET SINGLE_USER WITH ROLLBACK IMMEDIATE; "+
			"RESTORE DATABASE [%s] FROM DISK = N'%s' WITH REPLACE; "+
			"ALTER DATABASE [%s] SET MULTI_USER;",
		restorePlan.Database,
		restorePlan.Database,
		backupPath,
		restorePlan.Database,
	)

	var cmd *exec.Cmd
	if restorePlan.RequiresAuth {
		cmd = exec.Command("sqlcmd", "-S", server, "-U", user, "-P", password, "-Q", restoreQuery, "-b")
	} else {
		cmd = exec.Command("sqlcmd", "-S", server, "-E", "-Q", restoreQuery, "-b")
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := stderr.String()
		if errMsg == "" {
			errMsg = err.Error()
		}
		return fmt.Errorf("failed to rollback MSSQL database: %v\n%s", err, errMsg)
	}

	logger.Info("MSSQL rollback completed successfully")
	return nil
}
