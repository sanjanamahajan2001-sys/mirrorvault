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

func (m TUIModel) updateExecute(msg tea.Msg) (tea.Model, tea.Cmd) {

	switch msg := msg.(type) {

	case execProgressMsg:

		for i := range m.Exec.Items {
			item := &m.Exec.Items[i]
			if item.Database == msg.Database {

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
			// Stop polling when all items are done
			return m, nil
		}

		// Continue polling for more progress messages
		return m, execTick()

	case tea.KeyMsg:
		if m.Exec.AwaitExit && msg.String() == "enter" {
			return m, tea.Quit
		}
	}

	// For any other message type, continue polling
	return m, execTick()
}
