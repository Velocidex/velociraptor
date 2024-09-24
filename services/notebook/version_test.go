package notebook_test

import (
	"fmt"
	"time"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func (self *NotebookManagerTestSuite) TestUpdateCellVersions() {
	defer utils.MockTime(utils.NewMockClock(time.Unix(10, 10)))()
	gen := utils.IncrementalIdGenerator(0)
	defer utils.SetIdGenerator(&gen)()

	notebook_manager, err := services.GetNotebookManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Create a notebook the usual way.
	var notebook *api_proto.NotebookMetadata
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		notebook, err = notebook_manager.NewNotebook(
			self.Ctx, "admin", &api_proto.NotebookMetadata{
				Name: "Test Versioned Notebook",
			})
		return err == nil
	})

	// Now update the cell multiple times. Although we use the same
	// cell data the output may change each time so we recalculate it
	// with a new version
	last_version := ""
	var cell *api_proto.NotebookCell

	inputs := make(map[string]string)

	for i := 0; i < 10; i++ {
		input := fmt.Sprintf("# Heading %v\n", i)

		cell, err = notebook_manager.UpdateNotebookCell(self.Ctx, notebook,
			"admin", &api_proto.NotebookCellRequest{
				NotebookId: notebook.NotebookId,
				CellId:     notebook.CellMetadata[0].CellId,
				Input:      input,
				Type:       "MarkDown",
			})
		assert.NoError(self.T(), err)

		// Make sure that older versions are always trimmed.
		assert.True(self.T(),
			len(cell.AvailableVersions) <= 3)

		// Ensure versions are incremented for each calculation
		assert.True(self.T(), cell.CurrentVersion > last_version,
			"%v >  %v ", cell.CurrentVersion, last_version)
		last_version = cell.CurrentVersion

		inputs[cell.CurrentVersion] = input
	}

	// Now revert the version of the last cell. Here the
	// AvailableVersions should be [11, 12, 13] and CurrentVersion 13
	available_versions := cell.AvailableVersions
	assert.Equal(self.T(), len(available_versions), 3)

	// The current version is at available_versions[2]
	assert.Equal(self.T(), available_versions[2], cell.CurrentVersion)

	// Reset to an older version. e.g. [11, 12, 13] and version 12
	reverted_cell, err := notebook_manager.RevertNotebookCellVersion(self.Ctx,
		notebook.NotebookId, cell.CellId, available_versions[1])
	assert.NoError(self.T(), err)

	// Make sure we got the right version back
	assert.Equal(self.T(),
		reverted_cell.Input, inputs[available_versions[1]])

	// Fetch the notebook again from storage.
	notebook, err = notebook_manager.GetNotebook(self.Ctx,
		notebook.NotebookId, false)
	assert.NoError(self.T(), err)

	// Make sure the cell switched to the older version.
	// Now: [11, 12, 13] and CurrentVersion 12
	assert.Equal(self.T(), notebook.CellMetadata[0].CurrentVersion,
		available_versions[1])

	// Now calculate the cell once more, this should remove future
	// Redo versions after version 12
	cell, err = notebook_manager.UpdateNotebookCell(self.Ctx, notebook,
		"admin", &api_proto.NotebookCellRequest{
			NotebookId: notebook.NotebookId,
			CellId:     notebook.CellMetadata[0].CellId,
			Input:      "# Heading Redo\n",
			Type:       "MarkDown",
		})
	assert.NoError(self.T(), err)

	// Cell version is advanced
	assert.True(self.T(),
		cell.CurrentVersion > notebook.CellMetadata[0].CurrentVersion)

	// Here AvailableVersions should be [11, 12, 14] and CurrentVersion 14
	assert.Equal(self.T(), len(cell.AvailableVersions), 3)

	// First 2 versions are unchanged
	assert.Equal(self.T(), cell.AvailableVersions[0],
		notebook.CellMetadata[0].AvailableVersions[0])

	assert.Equal(self.T(), cell.AvailableVersions[1],
		notebook.CellMetadata[0].AvailableVersions[1])

	// But the last version is advanced.
	assert.Equal(self.T(), cell.AvailableVersions[2],
		cell.CurrentVersion)

	// Now check we can get the current version using GetNotebookCell()
	new_cell, err := notebook_manager.GetNotebookCell(self.Ctx,
		notebook.NotebookId, cell.CellId, "" /* Specify no cell version */)

	// Fetches the current version.
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), new_cell.CurrentVersion, cell.CurrentVersion)

	// Now get arbitrary versions
	new_cell, err = notebook_manager.GetNotebookCell(self.Ctx,
		notebook.NotebookId, cell.CellId, available_versions[1])
	assert.NoError(self.T(), err)

	assert.NoError(self.T(), err)
	assert.Equal(self.T(), new_cell.CurrentVersion, available_versions[1])
	assert.True(self.T(), available_versions[1] != cell.CurrentVersion)
}
