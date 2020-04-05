package reporting

import "path"

func GetNotebookPath(notebook_id string) string {
	return path.Join("notebooks", notebook_id+".json")
}

func GetNotebookCellPath(notebook_id, notebook_cell_id string) string {
	return path.Join("notebooks", notebook_id, notebook_cell_id+".json")
}
