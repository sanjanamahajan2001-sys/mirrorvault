package tui

import (
	"fmt"
	"os"

	"mirrorvault/internal/backup/credentials"
	"mirrorvault/internal/backup/plan"
	"mirrorvault/pkg/model"

	tea "github.com/charmbracelet/bubbletea"
)

func (m TUIModel) Init() tea.Cmd {
	if m.ViewState == ViewExecute {
		return startExecutionCmd(m)
	}
	return nil
}

func (m TUIModel) currentEngine() *model.Database {
	if m.Selection.EngineIndex < 0 || m.Selection.EngineIndex >= len(m.ScanResult.Databases) {
		return nil
	}
	return &m.ScanResult.Databases[m.Selection.EngineIndex]
}

func Run(scan model.ScanResult, mode Mode) (bool, error) {
	m := TUIModel{
		ScanResult: scan,
		Mode:       mode,
		ViewState:  ViewScan,
		Selection:  NewSelectionState(),
	}

	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),
		tea.WithOutput(os.Stdout),
	)

	finalModel, err := p.Run()
	if err != nil {
		return false, err
	}

	result := finalModel.(TUIModel)
	return result.Ready, nil
}

func (m TUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {

	switch msg := msg.(type) {

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}

		switch m.ViewState {

		case ViewScan:
			if msg.String() == "enter" {
				m.ViewState = ViewSelectEngine
			}

		case ViewSelectEngine:
			switch msg.String() {
			case "up":
				if m.Selection.EngineIndex > 0 {
					m.Selection.EngineIndex--
				}
			case "down":
				if m.Selection.EngineIndex < len(m.ScanResult.Databases)-1 {
					m.Selection.EngineIndex++
				}
			case "enter":
				m.ViewState = ViewSelectDB
			}

		case ViewSelectDB:
			if msg.String() == "enter" {
				engine := m.currentEngine()
				if engine != nil {
					var dbs []string
					for name, ok := range m.Selection.SelectedDBs {
						if ok {
							dbs = append(dbs, name)
						}
					}
					
					// If no databases selected, can't proceed
					if len(dbs) == 0 {
						// Can't create exec state with no databases
						return m, nil
					}
					
					m.Exec = NewExecState(engine.Engine, dbs)
					
					// Build plan and collect credentials before execution
					if m.Mode == BackupMode {
						selection := m.ExportSelection()
						// Debug: check if selection is empty
						if len(selection) == 0 {
							for _, item := range m.Exec.Items {
								EmitExecProgress(
									item.Engine,
									item.Database,
									"",
									0,
									"failed",
									fmt.Errorf("no databases in selection"),
								)
							}
							m.ViewState = ViewExecute
							m.Exec.Done = true
							m.Exec.AwaitExit = true
							return m, execTick()
						}
						
						backupPlan, err := plan.Build(m.ScanResult, selection)
						if err != nil {
							// If plan building fails, emit error for all databases
							for _, item := range m.Exec.Items {
								EmitExecProgress(
									item.Engine,
									item.Database,
									"",
									0,
									"failed",
									fmt.Errorf("failed to build backup plan: %v", err),
								)
							}
							m.ViewState = ViewExecute
							m.Exec.Done = true
							m.Exec.AwaitExit = true
							return m, execTick()
						}
						
						// Collect credentials
						authCtx := credentials.NewContext()
						for _, eng := range backupPlan.Engines {
							if !eng.RequiresAuth {
								continue
							}
							
							password, err := credentials.Prompt(eng.Engine)
							if err != nil {
								// If credential collection fails, emit error for all databases
								for _, item := range m.Exec.Items {
									EmitExecProgress(
										item.Engine,
										item.Database,
										"",
										0,
										"failed",
										fmt.Errorf("failed to collect credentials: %v", err),
									)
								}
								m.ViewState = ViewExecute
								m.Exec.Done = true
								m.Exec.AwaitExit = true
								return m, execTick()
							}
							
							authCtx.Set(eng.Engine, password)
						}
						
						m.Plan = backupPlan
						m.Auth = authCtx
						
						// Verify plan is set before starting execution
						if m.Plan == nil {
							for _, item := range m.Exec.Items {
								EmitExecProgress(
									item.Engine,
									item.Database,
									"",
									0,
									"failed",
									fmt.Errorf("plan was nil after building"),
								)
							}
							m.ViewState = ViewExecute
							m.Exec.Done = true
							m.Exec.AwaitExit = true
							return m, execTick()
						}
					} else {
						// Scan mode - no plan needed, just mark as done
						m.Plan = nil
						m.Auth = nil
					}
					
					// Transition to execution view and start immediately
					// Don't wait for another key press - start execution right away
					m.ViewState = ViewExecute
					// Start execution and begin polling for progress messages
					// The plan should now be set, so startExecutionCmd will have access to it
					// Return immediately to start execution - this prevents any buffered
					// Enter key from password submission from being processed again
					return m, tea.Batch(startExecutionCmd(m), execTick())
				}
			}
			return m.updateDBSelect(msg)

		case ViewExecute:
			return m.updateExecute(msg)
		}

	default:
		// 🔥 ALL execution progress events land here
		if m.ViewState == ViewExecute {
			return m.updateExecute(msg)
		}
	}

	return m, nil
}

func (m TUIModel) View() string {
	switch m.ViewState {
	case ViewSelectEngine:
		return m.viewEngineSelect()
	case ViewSelectDB:
		return m.viewDBSelect()
	case ViewExecute:
		return m.viewExecute()
	default:
		return m.viewScan()
	}
}
