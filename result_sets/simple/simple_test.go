package simple_test

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"

	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/result_sets/simple"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"

	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

var test_cases = []struct {
	name      string
	start_row int64
	end_row   int64
}{
	{"Rows 0-2", 0, 2},
	{"Rows 10-12", 10, 12},
	{"Rows 45-75", 45, 75},
}

type ResultSetTestSuite struct {
	test_utils.TestSuite

	file_store         api.FileStore
	client_id, flow_id string
}

func (self *ResultSetTestSuite) SetupTest() {
	self.TestSuite.SetupTest()

	self.client_id = "C.12312"
	self.flow_id = "F.1232"
	self.file_store = file_store.GetFileStore(self.ConfigObj)
}

func (self *ResultSetTestSuite) TestResultSetSimple() {
	path_manager, err := artifacts.NewArtifactPathManager(
		self.Ctx, self.ConfigObj,
		self.client_id,
		self.flow_id,
		"Generic.Client.Info/BasicInformation")
	assert.NoError(self.T(), err)

	writer, err := result_sets.NewResultSetWriter(
		self.file_store, path_manager.Path(),
		json.DefaultEncOpts(), utils.SyncCompleter,
		result_sets.TruncateMode)
	assert.NoError(self.T(), err)

	// Write 5 rows separately
	for i := int64(0); i < 50; i++ {
		writer.Write(ordereddict.NewDict().
			Set("Row", i).
			Set("Foo", "Bar"))
	}

	writer.Close()

	//	test_utils.GetMemoryFileStore(self.T(), self.config_obj).Debug()

	result := ordereddict.NewDict()

	for _, test_case := range test_cases {
		rs_reader, err := result_sets.NewResultSetReader(
			self.file_store, path_manager.Path())
		assert.NoError(self.T(), err)
		defer rs_reader.Close()

		err = rs_reader.SeekToRow(test_case.start_row)
		assert.NoError(self.T(), err)

		count := int64(test_case.start_row)
		rows := make([]*ordereddict.Dict, 0)
		for row := range rs_reader.Rows(self.Ctx) {
			rows = append(rows, row)
			count++
			if count > test_case.end_row {
				break
			}
		}
		result.Set(test_case.name, rows)
	}

	goldie.Assert(self.T(), "TestResultSets", json.MustMarshalIndent(result))
}

func (self *ResultSetTestSuite) TestResultSetWriter() {
	// Write some flow logs.
	path_manager := paths.NewFlowPathManager(self.client_id, self.flow_id).Log()
	rs, err := result_sets.NewResultSetWriter(
		self.file_store, path_manager, nil, utils.SyncCompleter, true)
	assert.NoError(self.T(), err)
	rs.Write(ordereddict.NewDict().Set("Foo", 1))
	rs.Write(ordereddict.NewDict().Set("Foo", 2))
	rs.Write(ordereddict.NewDict().Set("Foo", 3))

	// Writes may not occur until the Close()
	rs.Close()

	// v := test_utils.GetMemoryFileStore(self.T(), self.config_obj)
	// utils.Debug(v)

	// Opening past the end of file should return an EOF error.
	rs_reader, err := result_sets.NewResultSetReader(self.file_store, path_manager)
	assert.NoError(self.T(), err)
	defer rs_reader.Close()

	// Reader should report its size.
	assert.Equal(self.T(), rs_reader.TotalRows(), int64(3))

	err = rs_reader.SeekToRow(10000)
	assert.Error(self.T(), err)

	// Read the rows back out from the start
	rows := simple.GetAllResults(rs_reader)
	assert.Equal(self.T(), len(rows), 3)
	value, _ := rows[0].GetInt64("Foo")
	assert.Equal(self.T(), value, int64(1))

	// Read the rows back out from the first row
	err = rs_reader.SeekToRow(1)
	assert.NoError(self.T(), err)

	rows = simple.GetAllResults(rs_reader)
	assert.Equal(self.T(), len(rows), 2)
	value, _ = rows[0].GetInt64("Foo")
	assert.Equal(self.T(), value, int64(2))
}

func (self *ResultSetTestSuite) TestResultSetUpdaterBulkJSONL() {
	// Write some flow logs.
	path_manager := paths.NewFlowPathManager(self.client_id, self.flow_id).Log()
	rs, err := result_sets.NewResultSetWriter(
		self.file_store, path_manager, nil, utils.SyncCompleter, true)
	assert.NoError(self.T(), err)
	rs.WriteJSONL([]byte("{\"Foo\": 10}\n{\"Foo\": 20}\n{\"Foo\": 30}\n"), 3)

	// Writes may not occur until the Close()
	rs.Close()

	rs, err = result_sets.NewResultSetWriter(
		self.file_store, path_manager, nil, utils.SyncCompleter,
		result_sets.AppendMode)
	assert.NoError(self.T(), err)

	// Update a row with a new record which is shorter than the old
	// record, new record will be slotted inside the existing record
	// space.
	err = rs.Update(1, ordereddict.NewDict().Set("Foo", 7))
	assert.NoError(self.T(), err)

	// Reading the rows should be fine.
	rs_reader, err := result_sets.NewResultSetReader(self.file_store, path_manager)
	assert.NoError(self.T(), err)
	defer rs_reader.Close()

	// Read the rows back out from the start
	rows := simple.GetAllResults(rs_reader)
	assert.Equal(self.T(), len(rows), 3)
	value, _ := rows[1].GetInt64("Foo")
	assert.Equal(self.T(), value, int64(7))

	// Now update the row with a record which is longer than the
	// existing record.
	err = rs.Update(1, ordereddict.NewDict().Set("Foo", "A very long string"))
	assert.NoError(self.T(), err)

	rs_reader, err = result_sets.NewResultSetReader(self.file_store, path_manager)
	assert.NoError(self.T(), err)
	defer rs_reader.Close()

	rows = simple.GetAllResults(rs_reader)
	assert.Equal(self.T(), len(rows), 3)
	value_str, _ := rows[1].GetString("Foo")
	assert.Equal(self.T(), value_str, "A very long string")

	// Test reading with seek
	err = rs_reader.SeekToRow(1)
	assert.NoError(self.T(), err)

	for row := range rs_reader.Rows(self.Ctx) {
		value, _ := row.GetString("Foo")
		assert.Equal(self.T(), value, "A very long string")
		break
	}
}

func (self *ResultSetTestSuite) TestResultSetUpdaterWithAppend() {
	// Write some flow logs.
	path_manager := paths.NewFlowPathManager(self.client_id, self.flow_id).Log()
	rs, err := result_sets.NewResultSetWriter(
		self.file_store, path_manager, nil, utils.SyncCompleter, true)
	assert.NoError(self.T(), err)
	rs.WriteJSONL([]byte("{\"Foo\": 10}\n{\"Foo\": 20}\n{\"Foo\": 30}\n"), 3)

	// Writes may not occur until the Close()
	rs.Close()

	rs, err = result_sets.NewResultSetWriter(
		self.file_store, path_manager, nil, utils.SyncCompleter,
		result_sets.AppendMode)
	assert.NoError(self.T(), err)

	// Update with a long string will push the new record to the end of the result set.
	err = rs.Update(1, ordereddict.NewDict().Set("Foo", "A very long string"))
	assert.NoError(self.T(), err)

	// Append a new row to the end of the result_set.
	rs, err = result_sets.NewResultSetWriter(
		self.file_store, path_manager, nil, utils.SyncCompleter,
		result_sets.AppendMode)
	assert.NoError(self.T(), err)

	rs.WriteJSONL([]byte("{\"Foo\": \"Additional Row\"}\n"), 3)
	rs.Close()

	// Reading the rows should be fine.
	rs_reader, err := result_sets.NewResultSetReader(self.file_store, path_manager)
	assert.NoError(self.T(), err)
	defer rs_reader.Close()

	// Read the rows back out from the start
	rows := simple.GetAllResults(rs_reader)
	assert.Equal(self.T(), len(rows), 4)
	value_str, _ := rows[3].GetString("Foo")
	assert.Equal(self.T(), value_str, "Additional Row")
}

func (self *ResultSetTestSuite) TestResultSetUpdaterSeparateRows() {
	// Write some flow logs.
	path_manager := paths.NewFlowPathManager(self.client_id, self.flow_id).Log()
	rs, err := result_sets.NewResultSetWriter(
		self.file_store, path_manager, nil, utils.SyncCompleter, true)
	assert.NoError(self.T(), err)
	rs.Write(ordereddict.NewDict().Set("Foo", 10))
	rs.Write(ordereddict.NewDict().Set("Foo", 20))
	rs.Write(ordereddict.NewDict().Set("Foo", 30))

	// Writes may not occur until the Close()
	rs.Close()

	rs, err = result_sets.NewResultSetWriter(
		self.file_store, path_manager, nil, utils.SyncCompleter,
		result_sets.AppendMode)
	assert.NoError(self.T(), err)

	// Update a row with a new record which is shorter than the old
	// record, new record will be slotted inside the existing record
	// space.
	err = rs.Update(1, ordereddict.NewDict().Set("Foo", 7))
	assert.NoError(self.T(), err)

	// Reading the rows should be fine.
	rs_reader, err := result_sets.NewResultSetReader(self.file_store, path_manager)
	assert.NoError(self.T(), err)
	defer rs_reader.Close()

	// Read the rows back out from the start
	rows := simple.GetAllResults(rs_reader)
	assert.Equal(self.T(), len(rows), 3)
	value, _ := rows[1].GetInt64("Foo")
	assert.Equal(self.T(), value, int64(7))

	// Now update the row with a record which is longer than the
	// existing record.
	err = rs.Update(1, ordereddict.NewDict().Set("Foo", "A very long string"))
	assert.NoError(self.T(), err)

	rs_reader, err = result_sets.NewResultSetReader(self.file_store, path_manager)
	assert.NoError(self.T(), err)
	defer rs_reader.Close()

	rows = simple.GetAllResults(rs_reader)
	assert.Equal(self.T(), len(rows), 3)
	value_str, _ := rows[1].GetString("Foo")
	assert.Equal(self.T(), value_str, "A very long string")
}

// Make sure the ResultSetWriter completes properly.
func (self *ResultSetTestSuite) TestResultSetWriterWithCompletion() {
	// Write some flow logs.
	var mu sync.Mutex
	result := ordereddict.NewDict()

	path_manager := paths.NewFlowPathManager(self.client_id, self.flow_id).Log()
	rs, err := result_sets.NewResultSetWriter(self.file_store, path_manager,
		nil,
		func() {
			mu.Lock()
			result.Set("Ran", true)
			mu.Unlock()
		},
		true)
	assert.NoError(self.T(), err)

	rs.Write(ordereddict.NewDict().Set("Foo", 1))
	rs.Write(ordereddict.NewDict().Set("Foo", 2))
	rs.Write(ordereddict.NewDict().Set("Foo", 3))

	// Writes may not occur until the Close()
	rs.Close()

	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		mu.Lock()
		defer mu.Unlock()

		_, pres := result.Get("Ran")
		return pres
	})
}

func (self *ResultSetTestSuite) TestResultSetWriterTruncate() {
	// Write some flow logs.
	path_manager := paths.NewFlowPathManager(self.client_id, self.flow_id).Log()
	rs, err := result_sets.NewResultSetWriter(self.file_store,
		path_manager, nil, utils.SyncCompleter, false /* truncate */)
	assert.NoError(self.T(), err)
	rs.Write(ordereddict.NewDict().Set("Foo", 1))
	rs.Write(ordereddict.NewDict().Set("Foo", 2))
	rs.Write(ordereddict.NewDict().Set("Foo", 3))

	// Writes may not occur until the Close()
	rs.Close()

	rs, err = result_sets.NewResultSetWriter(self.file_store, path_manager,
		nil, utils.SyncCompleter, result_sets.TruncateMode)
	assert.NoError(self.T(), err)
	rs.Write(ordereddict.NewDict().Set("Foo", 4))
	rs.Write(ordereddict.NewDict().Set("Foo", 5))
	rs.Write(ordereddict.NewDict().Set("Foo", 6))

	// Writes may not occur until the Close()
	rs.Close()

	// Append some data
	rs, err = result_sets.NewResultSetWriter(self.file_store, path_manager,
		nil, utils.SyncCompleter, result_sets.AppendMode)
	assert.NoError(self.T(), err)
	rs.Write(ordereddict.NewDict().Set("Foo", 7))
	rs.Write(ordereddict.NewDict().Set("Foo", 8))
	rs.Write(ordereddict.NewDict().Set("Foo", 9))

	// Writes may not occur until the Close()
	rs.Close()

	// Read the rows back out from the first row 2 + 3
	rs_reader, err := result_sets.NewResultSetReader(self.file_store, path_manager)
	assert.NoError(self.T(), err)

	// Total rows should not include the truncated set.
	assert.Equal(self.T(), rs_reader.TotalRows(), int64(6))

	err = rs_reader.SeekToRow(1)
	assert.NoError(self.T(), err)

	rows := simple.GetAllResults(rs_reader)
	assert.Equal(self.T(), len(rows), 2+3)
	value, _ := rows[0].GetInt64("Foo")
	assert.Equal(self.T(), value, int64(5))
}

func (self *ResultSetTestSuite) TestResultSetWriterWriteJSONL() {
	// Use a new client id
	self.client_id = "C.12313"

	// WriteJSONL is supposed to optimize the write load by
	// writing large JSON chunks into the result set. We
	// deliberately do not want to parse it out so we just append
	// the data to the file. However we dont know any of the row
	// indexes in the JSON blob, but we do know how many rows it
	// is in total.
	path_manager := paths.NewFlowPathManager(self.client_id, self.flow_id).Log()
	rs, err := result_sets.NewResultSetWriter(self.file_store, path_manager,
		nil, utils.SyncCompleter, result_sets.AppendMode)
	assert.NoError(self.T(), err)
	rs.Write(ordereddict.NewDict().Set("Foo", 1))
	rs.WriteJSONL([]byte("{\"Foo\":2}\n{\"Foo\":3}\n"), 2)
	rs.Close()

	rs_reader, err := result_sets.NewResultSetReader(self.file_store, path_manager)
	assert.NoError(self.T(), err)

	// Total rows should include both packets.
	assert.Equal(self.T(), rs_reader.TotalRows(), int64(3))

	// Seek into the middle of the JSON blob (last row)
	err = rs_reader.SeekToRow(2)
	rows := simple.GetAllResults(rs_reader)
	assert.Equal(self.T(), len(rows), 1)
	value, _ := rows[0].GetInt64("Foo")
	assert.Equal(self.T(), value, int64(3))
}

func TestResultSets(t *testing.T) {
	suite.Run(t, &ResultSetTestSuite{})
}

type ResultSetTestSuiteFileBased struct {
	ResultSetTestSuite
	dir string
}

func (self *ResultSetTestSuiteFileBased) SetupTest() {
	self.ConfigObj = self.LoadConfig()

	var err error
	self.dir, err = tempfile.TempDir("file_store_test")
	assert.NoError(self.T(), err)

	self.ConfigObj.Datastore.Implementation = "FileBaseDataStore"
	self.ConfigObj.Datastore.FilestoreDirectory = self.dir
	self.ConfigObj.Datastore.Location = self.dir

	self.client_id = "C.12312"
	self.flow_id = "F.1232"
	self.file_store = file_store.GetFileStore(self.ConfigObj)

	self.TestSuite.SetupTest()
}

func (self *ResultSetTestSuiteFileBased) TearDownTest() {
	os.RemoveAll(self.dir)
}

func TestResultSetWriterFileBased(t *testing.T) {
	suite.Run(t, &ResultSetTestSuiteFileBased{
		ResultSetTestSuite: ResultSetTestSuite{},
	})
}
