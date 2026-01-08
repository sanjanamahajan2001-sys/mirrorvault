package main

import (
        "fmt"
        "os"
        "strings"

        "mirrorvault/internal/analyse"
        "mirrorvault/internal/backup/credentials"
        "mirrorvault/internal/backup/execute"
        "mirrorvault/internal/backup/plan"
        "mirrorvault/internal/output"
        "mirrorvault/internal/output/tui"
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
        case "schedule-daily":
                runScheduleDailyMode()
        case "list-schedules":
                runListSchedules()
        case "delete-schedule":
                runDeleteSchedule()
        case "cleanup":
                runCleanup()
        case "--version", "-v":
                fmt.Println(version.Version)
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

        // Execute backup
        if err := execute.Run(backupPlan, authCtx, func(engine, db, path string, size int64, status string, err error) {
                if err != nil {
                        fmt.Fprintf(os.Stderr, "Backup failed for %s/%s: %v\n", engine, db, err)
                } else {
                        fmt.Printf("Backup completed for %s/%s: %s (%d bytes)\n", engine, db, path, size)
                }
        }); err != nil {
                fmt.Fprintf(os.Stderr, "Backup execution failed: %v\n", err)
                os.Exit(1)
        }

        fmt.Println("Scheduled backup completed successfully")
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
                                Engine:    s.Engine,
                                Databases: s.Databases,
                                Time:      s.Time,
                        }
                        timerNames[i] = s.TimerName
                }

                // Create a simple TUI model just for viewing
                model := tui.TUIModel{
                        Schedules:          scheduleData,
                        ScheduleTimerNames: timerNames,
                        ScheduleIndex:      0,
                        ViewState:          tui.ViewScheduleList,
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

func printHelp() {
        fmt.Println(`MirrorVault — Secure Database Backup Agent

Usage:
  mirrorvault scan
  mirrorvault backup
  mirrorvault schedule-daily
  mirrorvault list-schedules
  mirrorvault delete-schedule <timer-name>
  mirrorvault delete-schedule --all
  mirrorvault cleanup
`)
}
