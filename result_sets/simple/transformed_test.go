package simple_test

import (
	"fmt"
	"regexp"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/result_sets/simple"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
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

func (self *ResultSetTestSuite) TestTransformFilter() {
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
	writer.Write(ordereddict.NewDict().
		Set("Str", "This is a string").
		Set("Int", 6))

	writer.Write(ordereddict.NewDict().
		Set("Str", "Another string").
		Set("Int", 12))

	writer.Write(ordereddict.NewDict().
		Set("Str", "Hello world").
		Set("Int", 12))

	// Missing column
	writer.Write(ordereddict.NewDict().
		Set("Str", nil).
		Set("Int", -10))

	writer.Close()

	get_rows := func(opts result_sets.ResultSetOptions) []*ordereddict.Dict {
		// Sort by Str and filter only rows with Str in them
		rs_reader, err := result_sets.NewResultSetReaderWithOptions(
			self.Ctx, self.ConfigObj,
			self.file_store, path_manager.Path(), opts)
		assert.NoError(self.T(), err)
		defer rs_reader.Close()

		// Read the rows back out from the start
		return simple.GetAllResults(rs_reader)
	}

	golden := ordereddict.NewDict()
	golden.Set("Filtered by Str Sort Asc",
		get_rows(result_sets.ResultSetOptions{
			SortColumn:   "Str",
			FilterColumn: "Str",
			FilterRegex:  regexp.MustCompile("string"),
			SortAsc:      true,
		}))

	golden.Set("Filtered out by Str Sort Asc",
		get_rows(result_sets.ResultSetOptions{
			SortColumn:    "Str",
			FilterColumn:  "Str",
			FilterRegex:   regexp.MustCompile("string"),
			FilterExclude: true,
			SortAsc:       true,
		}))

	// Non string values are stringified before the regex is applied.
	golden.Set("Filtered by Str - nil",
		get_rows(result_sets.ResultSetOptions{
			SortColumn:   "Str",
			FilterColumn: "Str",
			FilterRegex:  regexp.MustCompile("nil"),
			SortAsc:      true,
		}))

	// Integers are stringified before regex
	golden.Set("Filtered by Int Sort Asc",
		get_rows(result_sets.ResultSetOptions{
			SortColumn:   "Str",
			FilterColumn: "Int",
			FilterRegex:  regexp.MustCompile("1"),
			SortAsc:      true,
		}))

	goldie.Assert(self.T(), "TestTransformFilter",
		json.MustMarshalIndent(golden))
}
