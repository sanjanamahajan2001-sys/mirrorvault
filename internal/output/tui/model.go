package tui

import (
    "mirrorvault/internal/backup/credentials"
    "mirrorvault/internal/backup/plan"
    "mirrorvault/internal/restore/analyze"
    restoreplan "mirrorvault/internal/restore/plan"
    "mirrorvault/pkg/model"
)

type Mode int

const (
    ScanMode Mode = iota
    BackupMode
    RestoreMode
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
    // Restore-specific views
    ViewRestoreSelectEngine
    ViewRestoreSelectDB
    ViewRestoreDumpPath
    ViewRestoreConfirm
    ViewRestoreProgress
    ViewRestoreHistory
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

    // 🔥 REQUIRED FOR RESTORE
    RestorePlan        *restoreplan.RestorePlan
    RestoreDumpPath   string         // User-provided dump file path
    RestoreProgress   float64        // Progress percentage (0.0-1.0)
    RestoreStep       string         // Current restore step
    RestoreMessage    string         // Current restore message
    RestoreError      error          // Restore error if any
    PreRestoreStats   *analyze.DatabaseStats
    PostRestoreStats  *analyze.DatabaseStats
    RestoreBackupPath string         // Path to pre-restore backup
    RestoreScrollOffset int          // Scroll offset for restore progress view
    TerminalWidth      int           // Terminal width
    TerminalHeight     int           // Terminal height

    // Restore history
    RestoreHistory     []*RestoreHistoryItem
    RestoreHistoryIndex int          // Currently selected history index
    RestoreHistoryScrollOffset int    // Scroll offset for history view

    Ready bool
    Exit  bool
}

type RestoreHistoryItem struct {
    Timestamp        string
    Engine           string
    Database         string
    DumpPath         string
    DumpFormat       string
    Compressed       bool
    MultiDB          bool
    PreRestoreBackup string
    Success          bool
    RolledBack       bool
    Error            string
    LogFile          string
}
