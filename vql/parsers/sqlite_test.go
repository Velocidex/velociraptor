package parsers

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/jmoiron/sqlx"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/repository"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

type TestSuite struct {
	suite.Suite
	config_obj *config_proto.Config
	sm         *services.Service
}

func (self *TestSuite) SetupTest() {
	var err error
	self.config_obj, err = new(config.Loader).WithFileLoader(
		"../../http_comms/test_data/server.config.yaml").
		WithRequiredFrontend().WithWriteback().WithVerbose(true).
		LoadAndValidate()
	require.NoError(self.T(), err)

	self.config_obj.Frontend.DoNotCompressArtifacts = true

	// Start essential services.
	ctx, _ := context.WithTimeout(context.Background(), time.Second*60)
	self.sm = services.NewServiceManager(ctx, self.config_obj)

	require.NoError(self.T(), self.sm.Start(repository.StartRepositoryManager))
}

func (self *TestSuite) TearDownTest() {
	self.sm.Close()
	test_utils.GetMemoryFileStore(self.T(), self.config_obj).Clear()
	test_utils.GetMemoryDataStore(self.T(), self.config_obj).Clear()
}

func (self *TestSuite) createSqliteFile(filename string) error {
	handle, err := sqlx.Connect("sqlite3", filename+"?_locking=EXCLUSIVE")
	if err != nil {
		return err
	}
	//	defer handle.Close()

	_, err = handle.Exec("Create table foo(column1 int, column2 varchar(256))")
	assert.NoError(self.T(), err)

	_, err = handle.Exec("insert into foo(column1, column2) values (1, 'first')")
	assert.NoError(self.T(), err)

	return nil
}

func (self *TestSuite) TestSQLite() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	tempfile, err := ioutil.TempFile("", "sqlite")
	assert.NoError(self.T(), err)
	tempfile.Close()

	defer os.Remove(tempfile.Name())

	// Keep all the logging messages in a string so we can check
	// whats happening below.
	log_buffer := &strings.Builder{}

	builder := services.ScopeBuilder{
		Config:     self.config_obj,
		ACLManager: vql_subsystem.NullACLManager{},
		Logger:     log.New(log_buffer, "vql: ", 0),
		Env:        ordereddict.NewDict(),
	}

	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	scope := manager.BuildScope(builder)
	defer scope.Close()

	scope.Log("Creating sqlite file on %v\n", tempfile.Name())

	err = self.createSqliteFile(tempfile.Name())
	assert.NoError(self.T(), err)

	result := ordereddict.NewDict()
	plugin := _SQLitePlugin{}

	test_query := func(name, query string, args []interface{}) {
		rows := []vfilter.Row{}
		for row := range plugin.Call(ctx, scope, ordereddict.NewDict().
			Set("file", tempfile.Name()+"?_timeout=1").
			Set("args", args).
			Set("query", query)) {
			rows = append(rows, row)
		}
		result.Set(name, rows)
	}

	test_query("Simple SELECT", "SELECT * FROM foo", nil)
	test_query("Simple SELECT with args", "SELECT column1, column1 + ? FROM foo",
		[]interface{}{1})

	// Force scope to close to call destructors.
	scope.Close()

	// Since the file was locked we should have triggered a file
	// copy to a tempfile.
	assert.Contains(self.T(), log_buffer.String(), "creating a local copy")
	assert.Contains(self.T(), log_buffer.String(), "Using local copy")

	// Make sure the file was removed.
	assert.Contains(self.T(), log_buffer.String(), "removing tempfile")

	fmt.Println(log_buffer.String())
	goldie.Assert(self.T(), "TestSQLite", json.MustMarshalIndent(result))
}

func TestSQLite(t *testing.T) {
	suite.Run(t, &TestSuite{})
}
