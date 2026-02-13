package tui

import tea "github.com/charmbracelet/bubbletea"

var execChan = make(chan tea.Msg, 100)

func EmitExecProgress(
	engine, db, path string,
	size int64,
	status string,
	err error,
) {
	var st ExecStatus

	switch status {
	case "running":
		st = ExecRunning
	case "done":
		st = ExecDone
	default:
		st = ExecFailed
	}

	execChan <- execProgressMsg{
		Engine:   engine,
		Database: db,
		Status:   st,
		Path:     path,
		Size:     size,
		Err:      err,
	}
}

func EmitDriveProgress(
	engine, db, stage, message, remoteName string,
	backupSize, accountRemaining, accountTotal int64,
	err error,
) {
	execChan <- driveProgressMsg{
		Engine:            engine,
		Database:          db,
		Stage:             stage,
		Message:           message,
		RemoteName:        remoteName,
		BackupSize:        backupSize,
		AccountRemaining:  accountRemaining,
		AccountTotal:      accountTotal,
		Err:               err,
	}
}

func execTick() tea.Cmd {
	return func() tea.Msg {
		return <-execChan
	}
}
