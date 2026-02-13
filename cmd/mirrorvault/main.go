package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mirrorvault/internal/analyse"
	"mirrorvault/internal/backup/credentials"
	"mirrorvault/internal/backup/execute"
	"mirrorvault/internal/backup/plan"
	"mirrorvault/internal/logrotate"
	"mirrorvault/internal/output"
	"mirrorvault/internal/output/tui"
	restoreexecute "mirrorvault/internal/restore/execute"
	restorehistory "mirrorvault/internal/restore/history"
	restoreplan "mirrorvault/internal/restore/plan"
	"mirrorvault/internal/schedule"
	"mirrorvault/internal/version"
	"mirrorvault/pkg/model"

	"github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "scan":
		runScanMode()
	case "backup":
		runBackupMode()
	case "restore":
		runRestoreMode()
	case "restore-history":
		runRestoreHistory()
	case "schedule-daily":
		runScheduleDailyMode()
	case "list-schedules":
		runListSchedules()
	case "delete-schedule":
		runDeleteSchedule()
	case "fix-timers":
		runFixTimers()
	case "cleanup":
		runCleanup()
	case "install-logrotate":
		runInstallLogrotate()
	case "--version", "-v":
		fmt.Printf("Version: %s\n", version.Version)
		fmt.Printf("Commit: %s\n", version.Commit)
		fmt.Printf("Build Time: %s\n", version.BuildTime)
	default:
		printHelp()
		os.Exit(1)
	}
}

func runScanMode() {
	scanResult := analyse.ScanDatabases()

	if term.IsTerminal(int(os.Stdout.Fd())) {
		tuiModel, proceed, err := tui.RunWithModel(scanResult, tui.ScanMode)
		if err != nil || !proceed {
			return
		}

		// Scan mode = NO execution
		executePhase(scanResult, tuiModel, tui.ScanMode, true)
		return
	}

	output.PrintScanResult(scanResult)
}

func runBackupMode() {
	scanResult := analyse.ScanDatabases()

	backupFlags := flag.NewFlagSet("backup", flag.ContinueOnError)
	engineFlag := backupFlags.String("engine", "", "Database engine (e.g., MySQL)")
	allFlag := backupFlags.Bool("all", false, "Backup all databases for the selected engine(s)")
	passwordFlag := backupFlags.String("password", "", "Password for authenticated engines")
	passwordFileFlag := backupFlags.String("password-file", "", "Path to password file")
	var dbFlags stringSlice
	backupFlags.Var(&dbFlags, "db", "Database name (repeatable or comma-separated)")
	_ = backupFlags.Parse(os.Args[2:])

	flagsUsed := *engineFlag != "" || *allFlag || *passwordFlag != "" || *passwordFileFlag != "" || len(dbFlags) > 0
	if flagsUsed {
		runBackupNonTUI(scanResult, *engineFlag, dbFlags, *allFlag, *passwordFlag, *passwordFileFlag)
		return
	}

	// Check if this is a scheduled backup (non-interactive)
	if os.Getenv("MIRRORVAULT_SCHEDULED") == "true" {
		runScheduledBackup(scanResult)
		return
	}

	if term.IsTerminal(int(os.Stdout.Fd())) {
		// TUI mode: everything is handled inside the TUI
		// Credentials are collected, plan is built, and execution happens in the TUI
		_, _, err := tui.RunWithModel(scanResult, tui.BackupMode)
		if err != nil {
			return
		}
		// TUI handles everything, no need to call executePhase
		return
	}

	// Non-TUI fallback
	executePhase(scanResult, tui.TUIModel{}, tui.BackupMode, false)
}

func runScheduledBackup(scanResult model.ScanResult) {
	// Get scheduled engine and databases from environment
	scheduledEngine := os.Getenv("MIRRORVAULT_SCHEDULED_ENGINE")
	scheduledDBsStr := os.Getenv("MIRRORVAULT_SCHEDULED_DBS")

	if scheduledEngine == "" || scheduledDBsStr == "" {
		fmt.Fprintf(os.Stderr, "Error: MIRRORVAULT_SCHEDULED_ENGINE or MIRRORVAULT_SCHEDULED_DBS not set\n")
		os.Exit(1)
	}

	// Parse database names (space-separated)
	scheduledDBs := strings.Fields(scheduledDBsStr)

	// Build selection map
	selection := map[string][]string{
		scheduledEngine: scheduledDBs,
	}

	// Build backup plan
	backupPlan, err := plan.Build(scanResult, selection)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to build backup plan: %v\n", err)
		os.Exit(1)
	}

	if os.Getenv("MIRRORVAULT_SCHEDULED_CATCHUP") == "true" {
		if scheduledBackupsUpToDate(backupPlan) {
			fmt.Println("Scheduled backups are up to date; skipping catch-up run")
			return
		}
	}

	// Collect credentials if needed
	authCtx := credentials.NewContext()
	for _, engine := range backupPlan.Engines {
		if !engine.RequiresAuth {
			continue
		}

		// For scheduled backups, try to get password from environment variable
		// Format: MIRRORVAULT_<ENGINE>_PASSWORD
		envVar := fmt.Sprintf("MIRRORVAULT_%s_PASSWORD", strings.ToUpper(engine.Engine))
		password := os.Getenv(envVar)

		if password == "" {
			fmt.Fprintf(os.Stderr, "Error: Password required for %s but %s environment variable not set\n", engine.Engine, envVar)
			os.Exit(1)
		}

		authCtx.Set(engine.Engine, password)
	}

	// Execute backup (scheduled backups are local-only)
	if err := execute.Run(backupPlan, authCtx, func(engine, db, path string, size int64, status string, err error) {
		if err != nil {
			fmt.Fprintf(os.Stderr, "Backup failed for %s/%s: %v\n", engine, db, err)
			return
		}
		if status == "done" {
			fmt.Printf("Validation OK for %s/%s\n", engine, db)
			fmt.Printf("Backup completed for %s/%s: %s (%d bytes)\n", engine, db, path, size)
		}
	}, nil, false); err != nil {
		fmt.Fprintf(os.Stderr, "Backup execution failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Scheduled backup completed successfully")
}

func scheduledBackupsUpToDate(backupPlan *plan.BackupPlan) bool {
	today := time.Now().Format("2006-01-02")
	for _, engine := range backupPlan.Engines {
		if engine.OutputDir == "" {
			return false
		}
		for _, db := range engine.Databases {
			prefix := scheduledBackupPrefix(engine.Engine, db.Name)
			if !hasBackupForDate(engine.OutputDir, prefix, today) {
				return false
			}
		}
	}
	return true
}

func scheduledBackupPrefix(engine, dbName string) string {
	switch engine {
	case "SQLite":
		base := strings.TrimSuffix(filepath.Base(dbName), filepath.Ext(dbName))
		if base == "" {
			return dbName
		}
		return base
	case "Redis":
		if dbName == "dump.rdb" {
			return "redis"
		}
		if strings.HasSuffix(dbName, ".rdb") {
			return strings.TrimSuffix(dbName, ".rdb")
		}
		return dbName
	default:
		return dbName
	}
}

func hasBackupForDate(dir, prefix, date string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	expectedPrefix := fmt.Sprintf("%s_%s", prefix, date)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), expectedPrefix) {
			return true
		}
	}
	return false
}

func executePhase(
	scanResult model.ScanResult,
	tuiModel tui.TUIModel,
	mode tui.Mode,
	tuiUsed bool,
) {
	selection := tuiModel.ExportSelection()

	backupPlan, err := plan.Build(scanResult, selection)
	if err != nil {
		fmt.Println("Failed to build backup plan:", err)
		return
	}

	// Scan mode stops here
	if mode == tui.ScanMode {
		return
	}

	// 🔥 HAND OFF TO TUI (EXECUTION OWNED BY TUI)
	// Note: Credentials are collected INSIDE the TUI, not here
	// The TUI handles credential collection when user selects databases
	if tuiUsed {
		// The TUI already has Plan and Auth set, we don't need to set them here
		// This function is only called for non-TUI mode or scan mode
		return
	}

	// Non-TUI mode: collect credentials here
	authCtx := credentials.NewContext()
	for _, engine := range backupPlan.Engines {
		if !engine.RequiresAuth {
			continue
		}

		password, err := credentials.Prompt(engine.Engine)
		if err != nil {
			return
		}

		authCtx.Set(engine.Engine, password)
	}

	// For non-TUI mode, we would execute here, but that's not implemented yet
	// tuiModel.Plan = backupPlan
	// tuiModel.Auth = authCtx
}

func printPlan(p *plan.BackupPlan) {
	fmt.Println("\nBackup plan generated:\n")

	for _, engine := range p.Engines {
		fmt.Printf("• %s (%s)\n", engine.Engine, engine.Version)
		fmt.Printf("  Output directory: %s\n", engine.OutputDir)

		for _, db := range engine.Databases {
			fmt.Printf("    - %s\n", db.Name)
		}
		fmt.Println()
	}
}

func runScheduleDailyMode() {
	scanResult := analyse.ScanDatabases()

	scheduleFlags := flag.NewFlagSet("schedule-daily", flag.ContinueOnError)
	engineFlag := scheduleFlags.String("engine", "", "Database engine (e.g., MySQL)")
	timeFlag := scheduleFlags.String("time", "", "Time in HH:MM (24-hour)")
	allFlag := scheduleFlags.Bool("all", false, "Schedule all databases for the selected engine")
	passwordFlag := scheduleFlags.String("password", "", "Password for authenticated engines")
	passwordFileFlag := scheduleFlags.String("password-file", "", "Path to password file")
	var dbFlags stringSlice
	scheduleFlags.Var(&dbFlags, "db", "Database name (repeatable or comma-separated)")
	_ = scheduleFlags.Parse(os.Args[2:])

	flagsUsed := *engineFlag != "" || *timeFlag != "" || *allFlag || *passwordFlag != "" || *passwordFileFlag != "" || len(dbFlags) > 0
	if flagsUsed {
		runScheduleDailyNonTUI(scanResult, *engineFlag, dbFlags, *timeFlag, *allFlag, *passwordFlag, *passwordFileFlag)
		return
	}

	if term.IsTerminal(int(os.Stdout.Fd())) {
		// TUI mode: everything is handled inside the TUI
		_, _, err := tui.RunWithModel(scanResult, tui.ScheduleMode)
		if err != nil {
			return
		}
		return
	}

	// Non-TUI fallback not implemented for schedule-daily
	fmt.Println("schedule-daily requires an interactive terminal")
}

func runCleanup() {
	if err := schedule.RunCleanup(); err != nil {
		fmt.Printf("Error running cleanup: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Cleanup completed successfully")
}

func runListSchedules() {
	if term.IsTerminal(int(os.Stdout.Fd())) {
		// TUI mode: show schedules in a nice TUI
		schedules, err := schedule.GetAllSchedules()
		if err != nil {
			fmt.Printf("Error loading schedules: %v\n", err)
			return
		}

		// Convert to TUI format
		scheduleData := make([]tui.ScheduleData, len(schedules))
		timerNames := make([]string, len(schedules))
		for i, s := range schedules {
			scheduleData[i] = tui.ScheduleData{
				Engine:      s.Engine,
				Databases:   s.Databases,
				Time:        s.Time,
				Compression: s.Compression,
			}
			timerNames[i] = s.TimerName
		}

		// Create a simple TUI model just for viewing
		model := tui.TUIModel{
			Schedules:          scheduleData,
			ScheduleTimerNames: timerNames,
			ScheduleIndex:      0,
			ViewState:          tui.ViewScheduleList,
			Mode:               tui.ScheduleMode, // Set mode to prevent showing scan view
			Selection:          tui.NewSelectionState(),
		}

		// Run TUI to show schedules
		p := tea.NewProgram(model, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}
		return
	}

	// Non-TUI mode: simple text output
	schedules, err := schedule.GetAllSchedules()
	if err != nil {
		fmt.Printf("Error loading schedules: %v\n", err)
		return
	}

	if len(schedules) == 0 {
		fmt.Println("No scheduled backups found.")
		return
	}

	fmt.Println("Scheduled Backups:")
	fmt.Println("==================")
	for i, s := range schedules {
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("Engine: %s\n", s.Engine)
		fmt.Printf("Databases: %s\n", strings.Join(s.Databases, ", "))
		fmt.Printf("Time: %s\n", s.Time)
		fmt.Printf("Timer: %s\n", s.TimerName)
	}
}

func runDeleteSchedule() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: mirrorvault delete-schedule <timer-name>")
		fmt.Println("       mirrorvault delete-schedule --all  (delete all schedules)")
		fmt.Println("\nTo see timer names, run: mirrorvault list-schedules")
		return
	}

	if os.Args[2] == "--all" {
		if err := schedule.RemoveAllSchedules(); err != nil {
			fmt.Printf("Error removing all schedules: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("All schedules removed successfully")
		return
	}

	timerName := os.Args[2]
	if err := schedule.RemoveSchedule(timerName); err != nil {
		fmt.Printf("Error removing schedule: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Schedule %s removed successfully\n", timerName)
}

func runFixTimers() {
	fmt.Println("Fixing existing timer units...")
	if err := schedule.FixExistingTimers(); err != nil {
		fmt.Printf("Error fixing timers: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("All timer units have been fixed successfully!")
	fmt.Println("\nTo verify, run: systemctl list-timers --all | grep mirrorvault")
}

func runRestoreMode() {
	scanResult := analyse.ScanDatabases()

	restoreFlags := flag.NewFlagSet("restore", flag.ContinueOnError)
	engineFlag := restoreFlags.String("engine", "", "Database engine (e.g., PostgreSQL)")
	dbFlag := restoreFlags.String("db", "", "Target database name")
	dumpPathFlag := restoreFlags.String("dump-path", "", "Path to dump file")
	passwordFlag := restoreFlags.String("password", "", "Password for authenticated engines")
	passwordFileFlag := restoreFlags.String("password-file", "", "Path to password file")
	_ = restoreFlags.Parse(os.Args[2:])

	flagsUsed := *engineFlag != "" || *dbFlag != "" || *dumpPathFlag != "" || *passwordFlag != "" || *passwordFileFlag != ""
	if flagsUsed {
		runRestoreNonTUI(scanResult, *engineFlag, *dbFlag, *dumpPathFlag, *passwordFlag, *passwordFileFlag)
		return
	}

	if term.IsTerminal(int(os.Stdout.Fd())) {
		// TUI mode: everything is handled inside the TUI
		_, _, err := tui.RunWithModel(scanResult, tui.RestoreMode)
		if err != nil {
			return
		}
		return
	}

	// Non-TUI fallback not implemented for restore
	fmt.Println("restore requires an interactive terminal")
}

func runBackupNonTUI(
	scanResult model.ScanResult,
	engineStr string,
	dbs []string,
	all bool,
	password string,
	passwordFile string,
) {
	engines := parseEngines(engineStr)
	selection, err := buildSelection(scanResult, engines, dbs, all)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Backup selection error: %v\n", err)
		os.Exit(1)
	}

	backupPlan, err := plan.Build(scanResult, selection)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to build backup plan: %v\n", err)
		os.Exit(1)
	}

	authCtx := credentials.NewContext()
	for _, engine := range backupPlan.Engines {
		if !engine.RequiresAuth {
			continue
		}

		pwd, err := resolvePassword(engine.Engine, password, passwordFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Credential error: %v\n", err)
			os.Exit(1)
		}
		authCtx.Set(engine.Engine, pwd)
	}

	if err := execute.Run(backupPlan, authCtx, func(engine, db, path string, size int64, status string, err error) {
		if err != nil {
			fmt.Fprintf(os.Stderr, "Backup failed for %s/%s: %v\n", engine, db, err)
			return
		}
		if status == "done" {
			fmt.Printf("Validation OK for %s/%s\n", engine, db)
			fmt.Printf("Backup completed for %s/%s: %s (%d bytes)\n", engine, db, path, size)
		}
	}, nil, false); err != nil {
		fmt.Fprintf(os.Stderr, "Backup execution failed: %v\n", err)
		os.Exit(1)
	}
}

func runRestoreNonTUI(
	scanResult model.ScanResult,
	engine string,
	database string,
	dumpPath string,
	password string,
	passwordFile string,
) {
	if engine == "" || database == "" || dumpPath == "" {
		fmt.Fprintln(os.Stderr, "restore requires --engine, --db, and --dump-path")
		os.Exit(1)
	}

	restorePlan, err := restoreplan.Build(engine, database, dumpPath, scanResult)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to build restore plan: %v\n", err)
		os.Exit(1)
	}

	authCtx := credentials.NewContext()
	if restorePlan.RequiresAuth {
		pwd, err := resolvePassword(engine, password, passwordFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Credential error: %v\n", err)
			os.Exit(1)
		}
		authCtx.Set(engine, pwd)
	}

	_, err = restoreexecute.Run(restorePlan, authCtx, func(step string, progress float64, message string, err error) {
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", step, err)
			return
		}
		if message != "" {
			fmt.Printf("[%s] %s\n", step, message)
		} else {
			fmt.Printf("[%s] %.0f%%\n", step, progress*100)
		}
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Restore failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Restore completed successfully")
}

func runScheduleDailyNonTUI(
	scanResult model.ScanResult,
	engine string,
	dbs []string,
	time string,
	all bool,
	password string,
	passwordFile string,
) {
	if engine == "" || time == "" {
		fmt.Fprintln(os.Stderr, "schedule-daily requires --engine and --time")
		os.Exit(1)
	}

	selection, err := buildSelection(scanResult, []string{engine}, dbs, all)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Schedule selection error: %v\n", err)
		os.Exit(1)
	}

	selectedDBs := selection[engine]
	if len(selectedDBs) == 0 {
		fmt.Fprintln(os.Stderr, "No databases selected for scheduling")
		os.Exit(1)
	}

	requiresAuth := false
	for _, db := range scanResult.Databases {
		if db.Engine == engine {
			requiresAuth = db.RequiresAuth
			break
		}
	}

	if requiresAuth {
		pwd, err := resolvePassword(engine, password, passwordFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Credential error: %v\n", err)
			os.Exit(1)
		}
		password = pwd
	}

	if err := schedule.AddSchedule(engine, selectedDBs, time, "", password); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to schedule backup: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Schedule created successfully")
}

func runRestoreHistory() {
	if term.IsTerminal(int(os.Stdout.Fd())) {
		// Load restore history
		histories, err := restorehistory.LoadRestoreHistory()
		if err != nil {
			fmt.Printf("Error loading restore history: %v\n", err)
			os.Exit(1)
		}

		// Convert to TUI format
		historyItems := make([]*tui.RestoreHistoryItem, len(histories))
		for i, h := range histories {
			historyItems[i] = &tui.RestoreHistoryItem{
				Timestamp:        h.Timestamp.Format("2006-01-02 15:04:05"),
				Engine:           h.Engine,
				Database:         h.Database,
				DumpPath:         h.DumpPath,
				DumpFormat:       h.DumpFormat,
				Compressed:       h.Compressed,
				MultiDB:          h.MultiDB,
				PreRestoreBackup: h.PreRestoreBackup,
				Success:          h.Success,
				RolledBack:       h.RolledBack,
				Error:            h.Error,
				LogFile:          h.LogFile,
			}
		}

		// Create TUI model
		model := tui.TUIModel{
			RestoreHistory:             historyItems,
			RestoreHistoryIndex:        0,
			RestoreHistoryScrollOffset: 0,
			ViewState:                  tui.ViewRestoreHistory,
			Mode:                       tui.RestoreMode,
			Selection:                  tui.NewSelectionState(),
			TerminalWidth:              80,
			TerminalHeight:             24,
		}

		// Run TUI
		p := tea.NewProgram(model, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}
		return
	}

	// Non-TUI mode: simple text output
	histories, err := restorehistory.LoadRestoreHistory()
	if err != nil {
		fmt.Printf("Error loading restore history: %v\n", err)
		os.Exit(1)
	}

	if len(histories) == 0 {
		fmt.Println("No restore operations found.")
		return
	}

	fmt.Println("Restore History:")
	fmt.Println("===============")
	for i, h := range histories {
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("Restore #%d - %s\n", i+1, h.Timestamp.Format("2006-01-02 15:04:05"))
		fmt.Printf("  Engine: %s\n", h.Engine)
		fmt.Printf("  Database: %s\n", h.Database)
		fmt.Printf("  Dump: %s\n", h.DumpPath)
		fmt.Printf("  Format: %s", h.DumpFormat)
		if h.Compressed {
			fmt.Printf(" (compressed)")
		}
		if h.MultiDB {
			fmt.Printf(" (multi-DB)")
		}
		fmt.Println()
		if h.PreRestoreBackup != "" {
			fmt.Printf("  Pre-Restore Backup: %s\n", h.PreRestoreBackup)
		}
		if h.Success {
			fmt.Printf("  Status: ✓ SUCCESS\n")
		} else {
			fmt.Printf("  Status: ✗ FAILED\n")
			if h.Error != "" {
				fmt.Printf("  Error: %s\n", h.Error)
			}
		}
		if h.RolledBack {
			fmt.Printf("  Rolled Back: Yes\n")
		}
	}
}

func printHelp() {
	fmt.Println(`MirrorVault — Secure Database Backup Agent

Usage:
  mirrorvault scan
  mirrorvault backup
  mirrorvault restore
  mirrorvault restore-history
  mirrorvault schedule-daily
  mirrorvault list-schedules
  mirrorvault delete-schedule <timer-name>
  mirrorvault delete-schedule --all
  mirrorvault fix-timers
  mirrorvault cleanup
  mirrorvault install-logrotate
`)
}

func runInstallLogrotate() {
	if err := logrotate.Install(); err != nil {
		fmt.Printf("Error installing logrotate config: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Logrotate config installed successfully")
}
