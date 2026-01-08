package tui

import (
    "mirrorvault/internal/backup/credentials"
    "mirrorvault/internal/backup/plan"
    "mirrorvault/pkg/model"
)

type Mode int

const (
    ScanMode Mode = iota
    BackupMode
)

type ViewState int

const (
    ViewScan ViewState = iota
    ViewSelectEngine
    ViewSelectDB
    ViewExecute
)

type TUIModel struct {
    ScanResult model.ScanResult
    Mode       Mode

    ViewState ViewState
    Selection SelectionState
    Exec      ExecState

    // 🔥 REQUIRED FOR EXECUTION
    Plan *plan.BackupPlan
    Auth *credentials.AuthContext

    Ready bool
    Exit  bool
}
