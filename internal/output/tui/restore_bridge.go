package tui

import tea "github.com/charmbracelet/bubbletea"

var restoreChan = make(chan tea.Msg, 100)

type restoreProgressMsg struct {
	Step    string
	Progress float64
	Message string
	Error   error
}

type restoreCompleteMsg struct {
	Success          bool
	BackupPath       string
	PreRestoreStats  interface{}
	PostRestoreStats interface{}
	Error            error
	RolledBack       bool
}

func EmitRestoreProgress(step string, progress float64, message string, err error) {
	restoreChan <- restoreProgressMsg{
		Step:     step,
		Progress: progress,
		Message:  message,
		Error:    err,
	}
}

func restoreTick() tea.Cmd {
	return func() tea.Msg {
		return <-restoreChan
	}
}

func startRestoreCmd(m TUIModel) tea.Cmd {
	return tea.Batch(
		restoreTick(),
		func() tea.Msg {
			// This will be handled by the restore executor
			return nil
		},
	)
}
