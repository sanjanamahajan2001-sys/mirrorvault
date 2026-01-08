package tui

type SelectionState struct {
	EngineIndex int
	DBIndex     int
	// Store selections per engine: engine -> database -> selected
	SelectedDBs map[string]map[string]bool
}

func NewSelectionState() SelectionState {
	return SelectionState{
		EngineIndex: 0,
		DBIndex:     0,
		SelectedDBs: make(map[string]map[string]bool),
	}
}
