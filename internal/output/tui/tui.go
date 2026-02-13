package tui

import (
	"fmt"
	"os"

	"mirrorvault/internal/backup/credentials"
	"mirrorvault/internal/backup/plan"
	"mirrorvault/internal/drive"
	"mirrorvault/pkg/model"

	tea "github.com/charmbracelet/bubbletea"
)

func (m TUIModel) startBackupExecution() (tea.Model, tea.Cmd) {
	selection := m.ExportSelection()
	if len(selection) == 0 {
		return m, nil
	}

	var allExecItems []ExecItem
	for engineName, dbNames := range selection {
		if len(dbNames) == 1 && dbNames[0] == plan.AllDatabasesName {
			allExecItems = append(allExecItems, ExecItem{
				Engine:   engineName,
				Database: "All databases",
				Status:   ExecPending,
			})
			continue
		}
		for _, dbName := range dbNames {
			allExecItems = append(allExecItems, ExecItem{
				Engine:   engineName,
				Database: dbName,
				Status:   ExecPending,
			})
		}
	}

	m.Exec = ExecState{
		Items: allExecItems,
	}

	if m.Mode == BackupMode {
		if m.BackupCompression == "" {
			_ = os.Unsetenv("MV_BACKUP_COMPRESSION")
		} else {
			_ = os.Setenv("MV_BACKUP_COMPRESSION", m.BackupCompression)
		}

		backupPlan, err := plan.Build(m.ScanResult, selection)
		if err != nil {
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

		authCtx := credentials.NewContext()
		for _, eng := range backupPlan.Engines {
			if !eng.RequiresAuth {
				continue
			}

			password, err := credentials.Prompt(eng.Engine)
			if err != nil {
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
	}

	m.ViewState = ViewExecute
	return m, tea.Batch(startExecutionCmd(m), execTick())
}

func (m TUIModel) Init() tea.Cmd {
	if m.ViewState == ViewExecute {
		return startExecutionCmd(m)
	}
	if m.ViewState == ViewRestoreProgress {
		return startRestoreCmd(m)
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
	driveCfg, driveErr := drive.LoadConfig()
	if driveCfg == nil {
		driveCfg = &drive.Config{Provider: "google_drive"}
	}
	m := TUIModel{
		ScanResult:           scan,
		Mode:                 mode,
		ViewState:            ViewScan,
		Selection:            NewSelectionState(),
		DriveConfig:          driveCfg,
		DriveConfigLoadError: driveErr,
		DriveEnabled:         driveCfg != nil && driveCfg.Enabled && driveCfg.IsConfigured(),
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

	case tea.WindowSizeMsg:
		m.TerminalWidth = msg.Width
		m.TerminalHeight = msg.Height
		return m, nil

	case tea.KeyMsg:
		// Only handle global quit for ctrl+c, let views handle 'q' themselves
		// This allows 'q' to be typed in text input fields
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

		switch m.ViewState {
		case ViewDriveSetup:
			return m.updateDriveSetup(msg)
		case ViewDriveConnect:
			return m.updateDriveConnect(msg)
		case ViewDriveClientSetup:
			return m.updateDriveClientSetup(msg)
		case ViewDriveConnectMethod:
			return m.updateDriveConnectMethod(msg)
		case ViewDriveFolderSelect:
			return m.updateDriveFolderSelect(msg)
		case ViewDriveFolderCreate:
			return m.updateDriveFolderCreate(msg)
		case ViewRestoreSource:
			return m.updateRestoreSource(msg)
		case ViewDriveFileSelect:
			return m.updateDriveFileSelect(msg)
		case ViewDriveDownload:
			return m.updateDriveDownload(msg)

		case ViewRestoreSelectEngine:
			return m.updateRestoreSelectEngine(msg)
		case ViewRestoreSelectDB:
			return m.updateRestoreSelectDB(msg)
		case ViewRestoreDumpPath:
			return m.updateRestoreDumpPath(msg)
		case ViewRestoreConfirm:
			return m.updateRestoreConfirm(msg)
		case ViewRestoreProgress:
			return m.updateRestoreProgress(msg)
		case ViewRestoreHistory:
			return m.updateRestoreHistory(msg)

		case ViewScan:
			// In ScanMode, Enter should do nothing - only allow exit
			if msg.String() == "enter" && m.Mode != ScanMode {
				if m.Mode == BackupMode {
					m.ViewState = ViewDriveSetup
					return m, m.driveSetupInitCmd()
				}
				m.ViewState = ViewSelectEngine
			}
			return m, nil

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
				m.DBSelectScrollOffset = 0 // Reset scroll when entering DB selection
				m.Selection.DBIndex = 0    // Reset selection index
			}
			return m, nil

		case ViewSelectDB:
			if msg.String() == "enter" {
				// Export all selections from all engines
				selection := m.ExportSelection()

				// If no databases selected, can't proceed
				if len(selection) == 0 {
					// Can't create exec state with no databases
					return m, nil
				}

				// For ScheduleMode, transition to time input
				if m.Mode == ScheduleMode {
					// Get all selected engines and their databases
					// For now, we'll schedule one engine at a time
					// Start with the current engine
					currentEngine := m.currentEngine()
					if currentEngine == nil {
						return m, nil
					}

					// Get selected databases for current engine
					selectedDBs := []string{}
					if dbMap, ok := m.Selection.SelectedDBs[currentEngine.Engine]; ok {
						for dbName, selected := range dbMap {
							if selected {
								selectedDBs = append(selectedDBs, dbName)
							}
						}
					}

					if len(selectedDBs) == 0 {
						return m, nil
					}

					// Create schedule data
					m.ScheduleData = &ScheduleData{
						Engine:    currentEngine.Engine,
						Databases: selectedDBs,
						Time:      "",
					}
					m.ScheduleTime = ""
					m.ScheduleFormatIndex = 0
					m.ViewState = ViewScheduleFormatSelect
					return m, nil
				}

				if m.Mode == BackupMode {
					m.BackupFormatIndex = 0
					m.BackupCompression = ""
					m.ViewState = ViewBackupFormatSelect
					return m, nil
				}

				return m.startBackupExecution()
			}
			return m.updateDBSelect(msg)

		case ViewBackupFormatSelect:
			return m.updateBackupFormatSelect(msg)

		case ViewExecute:
			return m.updateExecute(msg)

		case ViewScheduleTime:
			return m.updateScheduleTime(msg)

		case ViewScheduleFormatSelect:
			return m.updateScheduleFormatSelect(msg)

		case ViewScheduleConfirm:
			return m.updateScheduleConfirm(msg)

		case ViewScheduleList:
			return m.updateScheduleList(msg)
		case ViewScheduleDuplicate:
			return m.updateScheduleDuplicate(msg)
		case ViewScheduleDeleteConfirm:
			return m.updateScheduleDeleteConfirm(msg)
		}

	default:
		if m2, cmd, handled := m.updateDriveMsg(msg); handled {
			return m2, cmd
		}
		// 🔥 ALL execution progress events land here (non-KeyMsg messages)
		if m.ViewState == ViewExecute {
			return m.updateExecute(msg)
		}
		// Handle restore progress messages
		if m.ViewState == ViewRestoreProgress {
			return m.updateRestoreProgressMsg(msg)
		}
	}

	return m, nil
}

func (m TUIModel) View() string {
	switch m.ViewState {
	case ViewDriveSetup:
		return m.viewDriveSetup()
	case ViewDriveConnect:
		return m.viewDriveConnect()
	case ViewDriveClientSetup:
		return m.viewDriveClientSetup()
	case ViewDriveConnectMethod:
		return m.viewDriveConnectMethod()
	case ViewDriveFolderSelect:
		return m.viewDriveFolderSelect()
	case ViewDriveFolderCreate:
		return m.viewDriveFolderCreate()
	case ViewRestoreSelectEngine:
		return m.viewRestoreSelectEngine()
	case ViewRestoreSelectDB:
		return m.viewRestoreSelectDB()
	case ViewRestoreSource:
		return m.viewRestoreSource()
	case ViewRestoreDumpPath:
		return m.viewRestoreDumpPath()
	case ViewRestoreConfirm:
		return m.viewRestoreConfirm()
	case ViewRestoreProgress:
		return m.viewRestoreProgress()
	case ViewRestoreHistory:
		return m.viewRestoreHistory()
	case ViewDriveFileSelect:
		return m.viewDriveFileSelect()
	case ViewDriveDownload:
		return m.viewDriveDownload()
	case ViewSelectEngine:
		return m.viewEngineSelect()
	case ViewSelectDB:
		return m.viewDBSelect()
	case ViewBackupFormatSelect:
		return m.viewBackupFormatSelect()
	case ViewExecute:
		return m.viewExecute()
	case ViewScheduleTime:
		return m.viewScheduleTime()
	case ViewScheduleFormatSelect:
		return m.viewScheduleFormatSelect()
	case ViewScheduleConfirm:
		return m.viewScheduleConfirm()
	case ViewScheduleList:
		return m.viewScheduleList()
	case ViewScheduleDuplicate:
		return m.viewScheduleDuplicate()
	case ViewScheduleDeleteConfirm:
		return m.viewScheduleDeleteConfirm()
	default:
		return m.viewScan()
	}
}
