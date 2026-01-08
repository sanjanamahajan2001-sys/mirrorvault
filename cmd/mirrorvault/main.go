package main

import (
        "fmt"
        "os"

        "mirrorvault/internal/analyse"
        "mirrorvault/internal/backup/credentials"
        "mirrorvault/internal/backup/plan"
        "mirrorvault/internal/output"
        "mirrorvault/internal/output/tui"
        "mirrorvault/internal/version"
        "mirrorvault/pkg/model"

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

func printHelp() {
        fmt.Println(`MirrorVault — Secure Database Backup Agent

Usage:
  mirrorvault scan
  mirrorvault backup
`)
}
