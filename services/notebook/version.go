package notebook

import (
	"context"
	"errors"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self *NotebookManager) RevertNotebookCellVersion(
	ctx context.Context, notebook_id, cell_id, version string) (
	*api_proto.NotebookCell, error) {
	current_cell, err := self.getCurrentCellVersion(notebook_id, cell_id)
	if err != nil {
		return nil, err
	}

	if !utils.InString(current_cell.AvailableVersions, version) {
		return nil, errors.New("Version not found")
	}

	// Get previous version
	previous_cell, err := self.Store.GetNotebookCell(notebook_id, cell_id, version)
	if err != nil {
		return nil, err
	}

	// Update the previous cell with the list of all the currently
	// available versions.
	previous_cell.CurrentVersion = version
	previous_cell.AvailableVersions = current_cell.AvailableVersions

	return previous_cell, self.Store.SetNotebookCell(
		notebook_id, previous_cell)
}

func (self *NotebookManager) getCurrentCellVersion(
	notebook_id, cell_id string) (*api_proto.NotebookCell, error) {
	notebook_metadata, err := self.Store.GetNotebook(notebook_id)
	if err != nil {
		return nil, err
	}

	for _, cell := range notebook_metadata.CellMetadata {
		if cell.CellId == cell_id {
			return cell, nil
		}
	}
	return nil, errors.New("CellID Not found")
}

// Checks for expired versions and trims the storage if
// AvailableVersions is too far behind the current version.  For
// example: CurrentVersion: 5 AvailableVersions: [1,2,3,4,5] and
// config_obj.Defaults.NotebookVersions = 2 will trim versions 1, 2,
// and 3 to give AvailableVersions [4, 5]
func (self *NotebookManager) trimOldCellVersions(
	ctx context.Context, config_obj *config_proto.Config,
	notebook_id, cell_id string,
	available_versions []string) ([]string, error) {

	// By default keep 5 versions.
	limit := int64(5)
	if config_obj != nil && config_obj.Defaults != nil &&
		config_obj.Defaults.NotebookVersions > 0 {
		limit = config_obj.Defaults.NotebookVersions
	}

	var err error
	number_of_versions := int64(len(available_versions))

	if number_of_versions > limit {
		to_delete := available_versions[:number_of_versions-limit]
		available_versions = available_versions[number_of_versions-limit:]

		// Now delete the versions.
		for _, del := range to_delete {
			err = self.Store.RemoveNotebookCell(ctx, config_obj,
				notebook_id, cell_id, del, IGNORE_REPORT)
		}
	}

	return available_versions, err
}

// Removes any redo versions ahead of the current version. For
// example: CurrentVersion: 3, AvailableVersions: [1,2,3,4,5] will
// remove all versions ahead of version 3 so 4 and 5 results in
// AvailableVersions: [1,2,3]
func (self *NotebookManager) trimRedoVersions(
	ctx context.Context, config_obj *config_proto.Config,
	notebook_id, cell_id, current_version string,
	available_versions []string) ([]string, error) {

	var err error

	for i, version := range available_versions {
		if version == current_version && i+1 < len(available_versions) {
			// Partition the AvailableVersions into two slices -
			// versions before and including the current version and
			// versions after and excluding the current version.
			after := available_versions[i+1:]
			before := available_versions[:i+1]

			// The versions after the current version are the redo
			// versions. They need to be wiped.
			if len(after) > 0 {
				available_versions = before

				// Now remove the versions that occured after the
				// current version.
				for _, version := range after {
					err = self.Store.RemoveNotebookCell(ctx, config_obj,
						notebook_id, cell_id, version, IGNORE_REPORT)
				}
			}
			return available_versions, err
		}
	}

	return available_versions, nil
}
