package tui

import (
        "os"

        "mirrorvault/pkg/model"

        tea "github.com/charmbracelet/bubbletea"
)

func RunWithModel(scan model.ScanResult, mode Mode) (TUIModel, bool, error) {
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
                return TUIModel{}, false, err
        }

        result := finalModel.(TUIModel)
        return result, result.Ready, nil
}
