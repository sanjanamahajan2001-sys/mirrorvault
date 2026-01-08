package tui

type SelectionState struct {
	EngineIndex int
	DBIndex     int
	SelectedDBs map[string]bool
}

func NewSelectionState() SelectionState {
	return SelectionState{
		EngineIndex: 0,
		DBIndex:     0,
		SelectedDBs: make(map[string]bool),
	}
}
