package result_sets_test

import (
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/mysql"
	"www.velocidex.com/golang/velociraptor/file_store/result_sets"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/paths"
)

var Factory = result_sets.ResultSetFactory{}

type ResultSetTestSuite struct {
	suite.Suite

	config_obj         *config_proto.Config
	file_store         api.FileStore
	client_id, flow_id string
}

func (self *ResultSetTestSuite) SetupTest() {
	var err error
	self.config_obj, err = new(config.Loader).WithFileLoader(
		"../../http_comms/test_data/server.config.yaml").
		WithRequiredFrontend().WithWriteback().
		LoadAndValidate()
	require.NoError(self.T(), err)

	self.client_id = "C.12312"
	self.flow_id = "F.1232"
	self.file_store = file_store.GetFileStore(self.config_obj)
}

func (self *ResultSetTestSuite) TearDownTest() {
	test_utils.GetMemoryFileStore(self.T(), self.config_obj).Clear()
}

func (self *ResultSetTestSuite) TestResultSetWriter() {
	// Write some flow logs.
	path_manager := paths.NewFlowPathManager(self.client_id, self.flow_id).Log()
	rs, err := Factory.NewResultSetWriter(self.file_store, path_manager, nil, true)
	assert.NoError(self.T(), err)
	rs.Write(ordereddict.NewDict().Set("Foo", 1))
	rs.Write(ordereddict.NewDict().Set("Foo", 2))
	rs.Write(ordereddict.NewDict().Set("Foo", 3))

	// Writes may not occur until the Close()
	rs.Close()

	// v := test_utils.GetMemoryFileStore(self.T(), self.config_obj)
	// utils.Debug(v)

	// Openning past the end of file should return an EOF error.
	rs_reader, err := Factory.NewResultSetReader(self.file_store, path_manager)
	assert.NoError(self.T(), err)
	defer rs_reader.Close()

	// Reader should report its size.
	assert.Equal(self.T(), rs_reader.TotalRows(), int64(3))

	err = rs_reader.SeekToRow(10000)
	assert.Error(self.T(), err, io.EOF)

	// Read the rows back out from the start
	rows := rs_reader.(*result_sets.ResultSetReaderImpl).GetAllResults()
	assert.Equal(self.T(), len(rows), 3)
	value, _ := rows[0].GetInt64("Foo")
	assert.Equal(self.T(), value, int64(1))

	// Read the rows back out from the first row
	err = rs_reader.SeekToRow(1)
	assert.NoError(self.T(), err)

	rows = rs_reader.(*result_sets.ResultSetReaderImpl).GetAllResults()
	assert.Equal(self.T(), len(rows), 2)
	value, _ = rows[0].GetInt64("Foo")
	assert.Equal(self.T(), value, int64(2))
}

func (self *ResultSetTestSuite) TestResultSetWriterTruncate() {
	// Write some flow logs.
	path_manager := paths.NewFlowPathManager(self.client_id, self.flow_id).Log()
	rs, err := Factory.NewResultSetWriter(self.file_store, path_manager, nil, false /* truncate */)
	assert.NoError(self.T(), err)
	rs.Write(ordereddict.NewDict().Set("Foo", 1))
	rs.Write(ordereddict.NewDict().Set("Foo", 2))
	rs.Write(ordereddict.NewDict().Set("Foo", 3))

	// Writes may not occur until the Close()
	rs.Close()

	rs, err = Factory.NewResultSetWriter(self.file_store, path_manager, nil, true /* truncate */)
	assert.NoError(self.T(), err)
	rs.Write(ordereddict.NewDict().Set("Foo", 4))
	rs.Write(ordereddict.NewDict().Set("Foo", 5))
	rs.Write(ordereddict.NewDict().Set("Foo", 6))

	// Writes may not occur until the Close()
	rs.Close()

	// Append some data
	rs, err = Factory.NewResultSetWriter(self.file_store, path_manager, nil, false /* truncate */)
	assert.NoError(self.T(), err)
	rs.Write(ordereddict.NewDict().Set("Foo", 7))
	rs.Write(ordereddict.NewDict().Set("Foo", 8))
	rs.Write(ordereddict.NewDict().Set("Foo", 9))

	// Writes may not occur until the Close()
	rs.Close()

	// Read the rows back out from the first row 2 + 3
	rs_reader, err := Factory.NewResultSetReader(self.file_store, path_manager)
	assert.NoError(self.T(), err)

	// Total rows should not include the truncated set.
	assert.Equal(self.T(), rs_reader.TotalRows(), int64(6))

	err = rs_reader.SeekToRow(1)
	assert.NoError(self.T(), err)

	rows := rs_reader.(*result_sets.ResultSetReaderImpl).GetAllResults()
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
	rs, err := Factory.NewResultSetWriter(self.file_store, path_manager, nil, false /* truncate */)
	assert.NoError(self.T(), err)
	rs.Write(ordereddict.NewDict().Set("Foo", 1))
	rs.WriteJSONL([]byte("{\"Foo\":2}\n{\"Foo\":3}\n"), 2)
	rs.Close()

	//v := test_utils.GetMemoryFileStore(self.T(), self.config_obj)
	//utils.Debug(v)

	rs_reader, err := Factory.NewResultSetReader(self.file_store, path_manager)
	assert.NoError(self.T(), err)

	// Total rows should include both packets.
	assert.Equal(self.T(), rs_reader.TotalRows(), int64(3))

	// Seek into the middle of the JSON blob (last row)
	err = rs_reader.SeekToRow(2)
	rows := rs_reader.(*result_sets.ResultSetReaderImpl).GetAllResults()
	assert.Equal(self.T(), len(rows), 1)
	value, _ := rows[0].GetInt64("Foo")
	assert.Equal(self.T(), value, int64(3))
}

func TestResultSetWriter(t *testing.T) {
	suite.Run(t, &ResultSetTestSuite{})
}

type ResultSetTestSuiteFileBased struct {
	ResultSetTestSuite
	dir string
}

func (self *ResultSetTestSuiteFileBased) SetupTest() {
	var err error
	self.config_obj, err = new(config.Loader).WithFileLoader(
		"../../http_comms/test_data/server.config.yaml").
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

type ResultSetTestSuiteMysql struct {
	ResultSetTestSuite
}

func (self *ResultSetTestSuiteMysql) SetupTest() {
	var err error
	self.config_obj, err = new(config.Loader).WithFileLoader(
		"../../datastore/test_data/mysql.config.yaml").
		LoadAndValidate()
	require.NoError(self.T(), err)

	self.client_id = "C.12312"
	self.flow_id = "F.1232"
	self.file_store, err = mysql.SetupTest(self.config_obj)
	if err != nil {
		self.T().Skipf("Unable to contact mysql - skipping: %v", err)
	}
}

func (self *ResultSetTestSuiteMysql) TearDownTest() {
}

func TestResultSetWriterMysql(t *testing.T) {
	suite.Run(t, &ResultSetTestSuiteMysql{
		ResultSetTestSuite: ResultSetTestSuite{},
	})
}
