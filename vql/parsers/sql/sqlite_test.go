//go:build cgo
// +build cgo

package sql_test

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	vsql "www.velocidex.com/golang/velociraptor/vql/parsers/sql"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
	vfilter "www.velocidex.com/golang/vfilter"

	_ "www.velocidex.com/golang/velociraptor/accessors/file"
	_ "www.velocidex.com/golang/velociraptor/accessors/ntfs"
	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

type TestSuite struct {
	test_utils.TestSuite
}

func (self *TestSuite) createSqliteFile(filename string) error {
	handle, err := sql.Open("sqlite3", filename+"?_locking=EXCLUSIVE")
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

	tempfile, err := tempfile.TempFile("sqlite")
	assert.NoError(self.T(), err)
	tempfile.Close()

	defer os.Remove(tempfile.Name())

	// Keep all the logging messages in a string so we can check
	// whats happening below.
	log_buffer := &strings.Builder{}

	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Logger:     log.New(log_buffer, "vql: ", 0),
		Env:        ordereddict.NewDict(),
	}

	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	scope := manager.BuildScope(builder)
	defer scope.Close()

	scope.Log("Creating sqlite file on %v\n", tempfile.Name())

	err = self.createSqliteFile(tempfile.Name())
	assert.NoError(self.T(), err)

	result := ordereddict.NewDict()
	plugin := vsql.SQLitePlugin{}

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

	// Make sure the file was removed.
	assert.Contains(self.T(), log_buffer.String(), "Removed tempfile")

	fmt.Println(log_buffer.String())
	goldie.Assert(self.T(), "TestSQLite", json.MustMarshalIndent(result))
}

func TestSQLite(t *testing.T) {
	suite.Run(t, &TestSuite{})
}
