package tui

import (
	"fmt"

	"mirrorvault/internal/backup/credentials"
	"mirrorvault/internal/restore/analyze"
	restoreexecute "mirrorvault/internal/restore/execute"
	restoreplan "mirrorvault/internal/restore/plan"
	"mirrorvault/internal/restore/validate"
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

func prepareRestorePlan(m *TUIModel) error {
	if m == nil {
		return fmt.Errorf("restore model is nil")
	}
	plan, err := buildRestorePlan(*m)
	if err != nil {
		m.RestoreError = fmt.Errorf("failed to build restore plan: %v", err)
		return err
	}
	m.RestorePlan = plan

	dumpInfo, err := validate.ValidateDump(plan.DumpPath)
	if err != nil {
		m.RestoreError = fmt.Errorf("dump file validation failed: %v", err)
		return err
	}
	if err := validate.ValidateFormatCompatibility(dumpInfo, plan.Engine); err != nil {
		m.RestoreError = err
		return err
	}

	m.RestoreError = nil
	preStats, err := analyze.AnalyzeDatabase(plan.Engine, plan.Database, plan.RequiresAuth, "")
	if err == nil {
		m.PreRestoreStats = preStats
	}
	return nil
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
