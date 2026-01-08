package tui

import (
    "fmt"
    
    "mirrorvault/internal/backup/credentials"
    "mirrorvault/internal/backup/execute"

    tea "github.com/charmbracelet/bubbletea"
)

func startExecutionCmd(m TUIModel) tea.Cmd {
    // Capture the plan and auth at the time the command is created
    planCopy := m.Plan
    authCopy := m.Auth
    execItems := m.Exec.Items
    modeCopy := m.Mode
    
    return func() tea.Msg {
        // Start execution in a goroutine immediately
        go func() {
            // Safety check: if Plan is nil in BackupMode, emit errors for all databases
            if modeCopy == BackupMode && planCopy == nil {
                for _, item := range execItems {
                    EmitExecProgress(
                        item.Engine,
                        item.Database,
                        "",
                        0,
                        "failed",
                        fmt.Errorf("backup plan not initialized (mode: %v, plan: %v)", modeCopy, planCopy != nil),
                    )
                }
                return
            }
            
            // If Auth is nil, create empty context (for databases that don't require auth)
            if authCopy == nil {
                authCopy = credentials.NewContext()
            }
            
            // Only run if we have a plan (BackupMode)
            // Note: If planCopy is not nil, we should execute regardless of mode
            // because the plan was built, which means we're in backup mode
            if planCopy != nil {
                // Emit "running" status for all databases immediately to show execution started
                for _, item := range execItems {
                    EmitExecProgress(
                        item.Engine,
                        item.Database,
                        "",
                        0,
                        "running",
                        nil,
                    )
                }
                
                // Execute the backup plan
                _ = execute.Run(
                    planCopy,
                    authCopy,
                    EmitExecProgress,
                )
            } else {
                // If we get here, something is wrong - emit errors
                for _, item := range execItems {
                    EmitExecProgress(
                        item.Engine,
                        item.Database,
                        "",
                        0,
                        "failed",
                        fmt.Errorf("backup plan is nil (mode: %v)", modeCopy),
                    )
                }
            }
        }()
        // Return nil immediately - the goroutine handles everything
        return nil
    }
}
