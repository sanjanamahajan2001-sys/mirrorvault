package tui

import (
	"mirrorvault/internal/backup/credentials"
	"mirrorvault/internal/backup/plan"
	"mirrorvault/internal/drive"
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
	ViewDriveSetup
	ViewDriveConnect
	ViewDriveClientSetup
	ViewDriveConnectMethod
	ViewDriveFolderSelect
	ViewDriveFolderCreate
	ViewSelectEngine
	ViewSelectDB
	ViewScheduleFormatSelect
	ViewBackupFormatSelect
	ViewExecute
	ViewScheduleTime
	ViewScheduleConfirm
	ViewScheduleList
	ViewScheduleDuplicate
	ViewScheduleDeleteConfirm
	// Restore-specific views
	ViewRestoreSelectEngine
	ViewRestoreSelectDB
	ViewRestoreSource
	ViewRestoreDumpPath
	ViewRestoreConfirm
	ViewRestoreProgress
	ViewRestoreHistory
	ViewDriveFileSelect
	ViewDriveDownload
)

type ScheduleData struct {
	Engine      string
	Databases   []string
	Time        string // Format: HH:MM (e.g., "02:30")
	Password    string // Password for auth-required engines (only for scheduled backups)
	Compression string
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
	ScheduleTime           string         // Current time input
	ScheduleData           *ScheduleData  // Current schedule being created
	Schedules              []ScheduleData // All confirmed schedules
	ScheduleTimerNames     []string       // Timer names for each schedule (for editing/deleting)
	ScheduleIndex          int            // Currently selected schedule index in list view
	DuplicateSchedules     []ScheduleData // Conflicting schedules when duplicates detected
	DuplicateTimerNames    []string       // Timer names for duplicate schedules
	PendingDeleteTimerName string         // Timer name pending deletion confirmation

	// 🔥 REQUIRED FOR RESTORE
	RestorePlan         *restoreplan.RestorePlan
	RestoreDumpPath     string  // User-provided dump file path
	RestoreProgress     float64 // Progress percentage (0.0-1.0)
	RestoreStep         string  // Current restore step
	RestoreMessage      string  // Current restore message
	RestoreError        error   // Restore error if any
	PreRestoreStats     *analyze.DatabaseStats
	PostRestoreStats    *analyze.DatabaseStats
	RestoreBackupPath   string // Path to pre-restore backup
	RestoreScrollOffset int    // Scroll offset for restore progress view
	TerminalWidth       int    // Terminal width
	TerminalHeight      int    // Terminal height

	// Restore history
	RestoreHistory             []*RestoreHistoryItem
	RestoreHistoryIndex        int    // Currently selected history index
	RestoreHistoryScrollOffset int    // Scroll offset for history view
	RestoreSource              string // local or drive
	RestoreSourceIndex         int    // 0 local, 1 drive

	// Database selection scrolling
	DBSelectScrollOffset int    // Scroll offset for database selection view
	BackupFormatIndex    int    // 0 native, 1 compressed
	BackupCompression    string // empty for native, or compression type
	ScheduleFormatIndex  int    // 0 native, 1 compressed

	// Google Drive integration
	DriveConfig             *drive.Config
	DriveConfigLoadError    error
	DriveEnabled            bool
	DriveConnectID          int
	DriveConnectInProgress  bool
	DriveConnectMessage     string
	DriveConnectError       error
	DriveConnectMethod      string
	DriveConnectMethodIndex int
	DriveBrowserSession     *drive.BrowserSession
	DriveBrowserAuthURL     string
	DriveUserCode           string
	DriveVerificationURL    string
	DriveDeviceCode         string
	DriveAccountRemaining   int64
	DriveAccountTotal       int64
	DriveFolders            []drive.FolderItem
	DriveFolderIndex        int
	DriveFolderSize         int64
	DriveFolderSizeLoading  bool
	DriveFolderError        error
	DriveNewFolderName      string
	DriveClientIDInput      string
	DriveClientSecretInput  string
	DriveClientFieldIndex   int
	DriveFiles              []drive.FileItem
	DriveFileIndex          int
	DriveDownloadInProgress bool
	DriveDownloadPath       string
	DriveDownloadError      error

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
