package tui

func (m TUIModel) ExportSelection() map[string][]string {
	result := make(map[string][]string)

	engine := m.currentEngine()
	if engine == nil {
		return result
	}

	for name, selected := range m.Selection.SelectedDBs {
		if selected {
			result[engine.Engine] = append(result[engine.Engine], name)
		}
	}

	return result
}
