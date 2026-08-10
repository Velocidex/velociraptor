package notebook_test

import (
	"www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/notebook"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

func (self *NotebookManagerTestSuite) TestNotebookRestore() {
	notebook_manaer_, err := services.GetNotebookManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	provider := notebook_manaer_.(*notebook.NotebookManager).BackupProvider
	cell := &proto.NotebookCell{
		Type: "vql",
		// Should not evaluate the string as a template.
		Input: "SELECT * FROM info() {{ add 1 1 }}",
	}
	err = provider.RenderCell_(self.Ctx, cell)
	assert.NoError(self.T(), err)
	goldie.Assert(self.T(), "TestNotebookRestore", []byte(cell.Output))
}
