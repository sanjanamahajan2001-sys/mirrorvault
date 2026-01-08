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
    ScheduleMode
)

type ViewState int

const (
    ViewScan ViewState = iota
    ViewSelectEngine
    ViewSelectDB
    ViewExecute
    ViewScheduleTime
    ViewScheduleConfirm
    ViewScheduleList
    ViewScheduleDuplicate
    ViewScheduleDeleteConfirm
)

type ScheduleData struct {
    Engine   string
    Databases []string
    Time     string // Format: HH:MM (e.g., "02:30")
    Password string // Password for auth-required engines (only for scheduled backups)
}

type TUIModel struct {
    ScanResult model.ScanResult
    Mode       Mode

    ViewState ViewState
    Selection SelectionState
    Exec      ExecState

    // 🔥 REQUIRED FOR EXECUTION
    Plan *plan.BackupPlan
    Auth *credentials.AuthContext

    // 🔥 REQUIRED FOR SCHEDULING
    ScheduleTime string        // Current time input
    ScheduleData *ScheduleData // Current schedule being created
    Schedules    []ScheduleData // All confirmed schedules
    ScheduleTimerNames []string  // Timer names for each schedule (for editing/deleting)
    ScheduleIndex int           // Currently selected schedule index in list view
    DuplicateSchedules []ScheduleData // Conflicting schedules when duplicates detected
    DuplicateTimerNames []string      // Timer names for duplicate schedules
    PendingDeleteTimerName string    // Timer name pending deletion confirmation

    Ready bool
    Exit  bool
}
