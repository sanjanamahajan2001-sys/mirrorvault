package tui

import (
	"fmt"

	"mirrorvault/internal/backup/credentials"
	restoreexecute "mirrorvault/internal/restore/execute"
	restoreplan "mirrorvault/internal/restore/plan"
)

func buildRestorePlan(m TUIModel) (*restoreplan.RestorePlan, error) {
	engine := m.currentEngine()
	if engine == nil {
		return nil, fmt.Errorf("no engine selected")
	}

	displayNames := filterDefaultDatabases(engine.Engine, engine.Names)
	if m.Selection.DBIndex < 0 || m.Selection.DBIndex >= len(displayNames) {
		return nil, fmt.Errorf("no database selected")
	}

	selectedDB := displayNames[m.Selection.DBIndex]

	if m.RestoreDumpPath == "" {
		return nil, fmt.Errorf("dump path not provided")
	}

	plan, err := restoreplan.Build(engine.Engine, selectedDB, m.RestoreDumpPath, m.ScanResult)
	if err != nil {
		return nil, err
	}

	return plan, nil
}

func startRestoreExecution(m TUIModel) {
	// Build restore plan
	plan, err := buildRestorePlan(m)
	if err != nil {
		EmitRestoreProgress("Error", 0.0, "", err)
		return
	}

	// Collect credentials if needed
	authCtx := credentials.NewContext()
	if plan.RequiresAuth {
		password, err := credentials.Prompt(plan.Engine)
		if err != nil {
			EmitRestoreProgress("Authentication failed", 0.0, "", err)
			return
		}
		authCtx.Set(plan.Engine, password)
	}

	// Run restore in goroutine
	go func() {
		result, err := restoreexecute.Run(plan, authCtx, func(step string, progress float64, message string, err error) {
			EmitRestoreProgress(step, progress, message, err)
		})

		if err != nil {
			restoreChan <- restoreCompleteMsg{
				Success: false,
				Error:   err,
			}
			return
		}

		if result != nil {
			restoreChan <- restoreCompleteMsg{
				Success:          result.Success,
				BackupPath:       result.PreRestoreBackup,
				PostRestoreStats: result.PostRestoreStats,
				Error:            result.Error,
				RolledBack:       result.RolledBack,
			}
		}
	}()
}
