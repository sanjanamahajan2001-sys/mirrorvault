package tui

import (
	"os"

	"mirrorvault/internal/drive"
	"mirrorvault/pkg/model"

	tea "github.com/charmbracelet/bubbletea"
)

func RunWithModel(scan model.ScanResult, mode Mode) (TUIModel, bool, error) {
	initialView := ViewScan
	if mode == RestoreMode {
		initialView = ViewRestoreSelectEngine
	}

	driveCfg, driveErr := drive.LoadConfig()
	if driveCfg == nil {
		driveCfg = &drive.Config{Provider: "google_drive"}
	}

	m := TUIModel{
		ScanResult:        scan,
		Mode:              mode,
		ViewState:         initialView,
		Selection:         NewSelectionState(),
		TerminalWidth:     80,  // Default width
		TerminalHeight:    24,  // Default height
		RestoreScrollOffset: 0,
		DBSelectScrollOffset: 0,
		DriveConfig:       driveCfg,
		DriveConfigLoadError: driveErr,
		DriveEnabled:      driveCfg != nil && driveCfg.Enabled && driveCfg.IsConfigured(),
	}

	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),
		tea.WithOutput(os.Stdout),
	)

	finalModel, err := p.Run()
	if err != nil {
		return TUIModel{}, false, err
	}

	result := finalModel.(TUIModel)
	return result, result.Ready, nil
}
