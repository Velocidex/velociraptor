package simple_test

import (
	"fmt"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/sebdah/goldie"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/result_sets/simple"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self *ResultSetTestSuite) TestTransformed() {
	path_manager, err := artifacts.NewArtifactPathManager(
		self.Ctx, self.ConfigObj, self.client_id, self.flow_id,
		"Generic.Client.Info/BasicInformation")
	assert.NoError(self.T(), err)

	writer, err := result_sets.NewResultSetWriter(
		self.file_store, path_manager.Path(),
		json.DefaultEncOpts(), utils.SyncCompleter,
		result_sets.TruncateMode)

	assert.NoError(self.T(), err)

	// Write some data
	for i := int64(0); i < 50; i++ {
		writer.Write(ordereddict.NewDict().
			Set("Bar", fmt.Sprintf("Bar%d.txt", i/5)).
			Set("Foo", fmt.Sprintf("Foo%d.txt", i)))
	}
	writer.Close()

	// Reading the rows should be fine.
	rs_reader, err := result_sets.NewResultSetReaderWithOptions(
		self.Ctx, self.ConfigObj,
		self.file_store, path_manager.Path(), result_sets.ResultSetOptions{
			SortColumn: "Bar",
			SortAsc:    true,
		})
	assert.NoError(self.T(), err)
	defer rs_reader.Close()

	// Read the rows back out from the start
	rows := simple.GetAllResults(rs_reader)
	assert.Equal(self.T(), rs_reader.TotalRows(), int64(50))

	golden := ordereddict.NewDict()

	stack_reader, err := result_sets.NewResultSetReaderWithOptions(
		self.Ctx, self.ConfigObj,
		self.file_store, rs_reader.Stacker(), result_sets.ResultSetOptions{})
	assert.NoError(self.T(), err)
	defer stack_reader.Close()

	rows = simple.GetAllResults(stack_reader)
	golden.Set("Stacked by Bar", rows)

	goldie.Assert(self.T(), "TestTransformed",
		json.MustMarshalIndent(golden))
}
