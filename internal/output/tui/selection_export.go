package tui

func (m TUIModel) ExportSelection() map[string][]string {
	result := make(map[string][]string)

	// Collect ALL selected databases from ALL engines
	for engineName, dbMap := range m.Selection.SelectedDBs {
		if dbMap == nil {
			continue
		}
		for dbName, selected := range dbMap {
			if selected {
				result[engineName] = append(result[engineName], dbName)
			}
		}
	}

	return result
}
