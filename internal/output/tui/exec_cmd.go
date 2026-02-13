package tui

import tea "github.com/charmbracelet/bubbletea"

type execProgressMsg struct {
	Engine   string
	Database string
	Status   ExecStatus
	Path     string
	Size     int64
	Err      error
}

type driveProgressMsg struct {
	Engine           string
	Database         string
	Stage            string
	Message          string
	RemoteName       string
	BackupSize       int64
	AccountRemaining int64
	AccountTotal     int64
	Err              error
}

func (m TUIModel) updateExecute(msg tea.Msg) (tea.Model, tea.Cmd) {

	switch msg := msg.(type) {

	case execProgressMsg:

		for i := range m.Exec.Items {
			item := &m.Exec.Items[i]
			if item.Engine == msg.Engine && item.Database == msg.Database {

				item.Status = msg.Status
				item.Path = msg.Path
				item.Size = msg.Size
				item.Err = msg.Err

				if msg.Status == ExecDone || msg.Status == ExecFailed {
					m.Exec.Index++
				}
			}
		}

		if m.Exec.Index >= len(m.Exec.Items) {
			m.Exec.Done = true
			m.Exec.AwaitExit = true
		}

		// Continue polling for more progress messages (Drive may still be updating)
		return m, execTick()

	case driveProgressMsg:
		for i := range m.Exec.Items {
			item := &m.Exec.Items[i]
			if item.Engine == msg.Engine && item.Database == msg.Database {
				item.DriveMessage = msg.Message
				item.DriveRemoteName = msg.RemoteName
				item.DriveErr = msg.Err
				item.DriveBackupSize = msg.BackupSize
				item.DriveAccountRemain = msg.AccountRemaining
				item.DriveAccountTotal = msg.AccountTotal
				switch msg.Stage {
				case "checking":
					item.DriveStatus = DriveChecking
				case "uploading":
					item.DriveStatus = DriveUploading
				case "done":
					item.DriveStatus = DriveDone
				case "skipped":
					item.DriveStatus = DriveSkipped
				case "failed":
					item.DriveStatus = DriveFailed
				default:
					item.DriveStatus = DriveNone
				}
			}
		}
		return m, execTick()

	case tea.KeyMsg:
		if m.Exec.AwaitExit && msg.String() == "enter" {
			return m, tea.Quit
		}
	}

	// For any other message type, continue polling
	return m, execTick()
}
