package simple_test

import (
	"context"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/result_sets"
	"www.velocidex.com/golang/velociraptor/file_store/result_sets/simple"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/inventory"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/services/notifications"
	"www.velocidex.com/golang/velociraptor/services/repository"
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
	suite.Suite

	config_obj         *config_proto.Config
	file_store         api.FileStore
	client_id, flow_id string
	sm                 *services.Service
}

func (self *ResultSetTestSuite) SetupTest() {
	var err error
	self.config_obj, err = new(config.Loader).WithFileLoader(
		"../../../http_comms/test_data/server.config.yaml").
		WithRequiredFrontend().WithWriteback().
		LoadAndValidate()
	require.NoError(self.T(), err)

	self.client_id = "C.12312"
	self.flow_id = "F.1232"
	self.file_store = file_store.GetFileStore(self.config_obj)

	// Start essential services.
	ctx, _ := context.WithTimeout(context.Background(), time.Second*60)
	self.sm = services.NewServiceManager(ctx, self.config_obj)

	require.NoError(self.T(), self.sm.Start(journal.StartJournalService))
	require.NoError(self.T(), self.sm.Start(notifications.StartNotificationService))
	require.NoError(self.T(), self.sm.Start(launcher.StartLauncherService))
	require.NoError(self.T(), self.sm.Start(inventory.StartInventoryService))
	require.NoError(self.T(), self.sm.Start(repository.StartRepositoryManager))
}

func (self *ResultSetTestSuite) TearDownTest() {
	test_utils.GetMemoryFileStore(self.T(), self.config_obj).Clear()
}

func (self *ResultSetTestSuite) TestResultSetSimple() {
	path_manager, err := artifacts.NewArtifactPathManager(
		self.config_obj,
		self.client_id,
		self.flow_id,
		"Generic.Client.Info/BasicInformation")
	assert.NoError(self.T(), err)

	writer, err := result_sets.NewResultSetWriter(
		self.file_store, path_manager, nil, true /* truncate */)
	assert.NoError(self.T(), err)

	// Write 5 rows separately
	for i := int64(0); i < 50; i++ {
		writer.Write(ordereddict.NewDict().
			Set("Row", i).
			Set("Foo", "Bar"))
	}

	writer.Close()

	ctx := context.Background()

	//	test_utils.GetMemoryFileStore(self.T(), self.config_obj).Debug()

	result := ordereddict.NewDict()

	for _, test_case := range test_cases {
		rs_reader, err := result_sets.NewResultSetReader(
			self.file_store, path_manager)
		assert.NoError(self.T(), err)
		defer rs_reader.Close()

		err = rs_reader.SeekToRow(test_case.start_row)
		assert.NoError(self.T(), err)

		count := int64(test_case.start_row)
		rows := make([]*ordereddict.Dict, 0)
		for row := range rs_reader.Rows(ctx) {
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
	rs, err := result_sets.NewResultSetWriter(self.file_store, path_manager, nil, true)
	assert.NoError(self.T(), err)
	rs.Write(ordereddict.NewDict().Set("Foo", 1))
	rs.Write(ordereddict.NewDict().Set("Foo", 2))
	rs.Write(ordereddict.NewDict().Set("Foo", 3))

	// Writes may not occur until the Close()
	rs.Close()

	// v := test_utils.GetMemoryFileStore(self.T(), self.config_obj)
	// utils.Debug(v)

	// Openning past the end of file should return an EOF error.
	rs_reader, err := result_sets.NewResultSetReader(self.file_store, path_manager)
	assert.NoError(self.T(), err)
	defer rs_reader.Close()

	// Reader should report its size.
	assert.Equal(self.T(), rs_reader.TotalRows(), int64(3))

	err = rs_reader.SeekToRow(10000)
	assert.Error(self.T(), err)

	// Read the rows back out from the start
	rows := rs_reader.(*simple.ResultSetReaderImpl).GetAllResults()
	assert.Equal(self.T(), len(rows), 3)
	value, _ := rows[0].GetInt64("Foo")
	assert.Equal(self.T(), value, int64(1))

	// Read the rows back out from the first row
	err = rs_reader.SeekToRow(1)
	assert.NoError(self.T(), err)

	rows = rs_reader.(*simple.ResultSetReaderImpl).GetAllResults()
	assert.Equal(self.T(), len(rows), 2)
	value, _ = rows[0].GetInt64("Foo")
	assert.Equal(self.T(), value, int64(2))
}

func (self *ResultSetTestSuite) TestResultSetWriterTruncate() {
	// Write some flow logs.
	path_manager := paths.NewFlowPathManager(self.client_id, self.flow_id).Log()
	rs, err := result_sets.NewResultSetWriter(self.file_store, path_manager, nil, false /* truncate */)
	assert.NoError(self.T(), err)
	rs.Write(ordereddict.NewDict().Set("Foo", 1))
	rs.Write(ordereddict.NewDict().Set("Foo", 2))
	rs.Write(ordereddict.NewDict().Set("Foo", 3))

	// Writes may not occur until the Close()
	rs.Close()

	rs, err = result_sets.NewResultSetWriter(self.file_store, path_manager, nil, true /* truncate */)
	assert.NoError(self.T(), err)
	rs.Write(ordereddict.NewDict().Set("Foo", 4))
	rs.Write(ordereddict.NewDict().Set("Foo", 5))
	rs.Write(ordereddict.NewDict().Set("Foo", 6))

	// Writes may not occur until the Close()
	rs.Close()

	// Append some data
	rs, err = result_sets.NewResultSetWriter(self.file_store, path_manager, nil, false /* truncate */)
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

	rows := rs_reader.(*simple.ResultSetReaderImpl).GetAllResults()
	assert.Equal(self.T(), len(rows), 2+3)
	value, _ := rows[0].GetInt64("Foo")
	assert.Equal(self.T(), value, int64(5))
}

func (self *ResultSetTestSuite) TestResultSetWriterWriteJSONL() {
	// WriteJSONL is supposed to optimize the write load by
	// writing large JSON chunks into the result set. We
	// deliberately do not want to parse it out so we just append
	// the data to the file. However we dont know any of the row
	// indexes in the JSON blob, but we do know how many rows it
	// is in total.
	path_manager := paths.NewFlowPathManager(self.client_id, self.flow_id).Log()
	rs, err := result_sets.NewResultSetWriter(self.file_store, path_manager, nil, false /* truncate */)
	assert.NoError(self.T(), err)
	rs.Write(ordereddict.NewDict().Set("Foo", 1))
	rs.WriteJSONL([]byte("{\"Foo\":2}\n{\"Foo\":3}\n"), 2)
	rs.Close()

	//v := test_utils.GetMemoryFileStore(self.T(), self.config_obj)
	//utils.Debug(v)

	rs_reader, err := result_sets.NewResultSetReader(self.file_store, path_manager)
	assert.NoError(self.T(), err)

	// Total rows should include both packets.
	assert.Equal(self.T(), rs_reader.TotalRows(), int64(3))

	// Seek into the middle of the JSON blob (last row)
	err = rs_reader.SeekToRow(2)
	rows := rs_reader.(*simple.ResultSetReaderImpl).GetAllResults()
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
	var err error
	self.config_obj, err = new(config.Loader).WithFileLoader(
		"../../../http_comms/test_data/server.config.yaml").
		WithRequiredFrontend().WithWriteback().
		LoadAndValidate()
	require.NoError(self.T(), err)

	self.dir, err = ioutil.TempDir("", "file_store_test")
	assert.NoError(self.T(), err)

	self.config_obj.Datastore.Implementation = "FileBaseDataStore"
	self.config_obj.Datastore.FilestoreDirectory = self.dir
	self.config_obj.Datastore.Location = self.dir

	self.client_id = "C.12312"
	self.flow_id = "F.1232"
	self.file_store = file_store.GetFileStore(self.config_obj)
}

func (self *ResultSetTestSuiteFileBased) TearDownTest() {
	os.RemoveAll(self.dir)
}

func TestResultSetWriterFileBased(t *testing.T) {
	suite.Run(t, &ResultSetTestSuiteFileBased{
		ResultSetTestSuite: ResultSetTestSuite{},
	})
}
