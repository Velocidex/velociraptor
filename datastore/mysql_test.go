package datastore

import (
	"database/sql"
	"fmt"
	"sort"
	"testing"

	"github.com/alecthomas/assert"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

type MysqlTestSuite struct {
	suite.Suite

	config_obj *config_proto.Config
	datastore  DataStore
}

func (self *MysqlTestSuite) SetupTest() {
	// Drop the database to start a new test.
	conn_string := fmt.Sprintf("%s:%s@tcp(%s)/",
		self.config_obj.Datastore.MysqlUsername,
		self.config_obj.Datastore.MysqlPassword,
		self.config_obj.Datastore.MysqlServer)

	db, err := sql.Open("mysql", conn_string)
	assert.NoError(self.T(), err)

	_, err = db.Exec(fmt.Sprintf("drop database if exists `%v`",
		self.config_obj.Datastore.MysqlDatabase))
	if err != nil {
		self.T().Skipf("Unable to contact mysql - skipping: %v", err)
		return
	}
	defer db.Close()

	_, err = initializeDatabase(self.config_obj)
	assert.NoError(self.T(), err)

	self.datastore, err = NewMySQLDataStore(self.config_obj)
	assert.NoError(self.T(), err)
}

func (self MysqlTestSuite) TestSetGetSubject() {
	message := &crypto_proto.GrrMessage{Source: "Server"}

	urn := "/a/b/c"
	err := self.datastore.SetSubject(self.config_obj, urn, message)
	assert.NoError(self.T(), err)

	read_message := &crypto_proto.GrrMessage{}
	err = self.datastore.GetSubject(self.config_obj, urn, read_message)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), message.Source, read_message.Source)

	// Not existing urn returns no error but an empty message
	err = self.datastore.GetSubject(self.config_obj, urn+"foo", read_message)
	assert.NoError(self.T(), err)

	// Delete the subject
	err = self.datastore.DeleteSubject(self.config_obj, urn)
	assert.NoError(self.T(), err)

	// It should now be cleared
	err = self.datastore.GetSubject(self.config_obj, urn, read_message)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), "", read_message.Source)
}

func (self MysqlTestSuite) TestListChildren() {
	message := &crypto_proto.GrrMessage{Source: "Server"}

	urn := "/a/b/c"
	err := self.datastore.SetSubject(self.config_obj, urn+"/1", message)
	assert.NoError(self.T(), err)

	err = self.datastore.SetSubject(self.config_obj, urn+"/2", message)
	assert.NoError(self.T(), err)

	err = self.datastore.SetSubject(self.config_obj, urn+"/3", message)
	assert.NoError(self.T(), err)

	children, err := self.datastore.ListChildren(self.config_obj, urn, 0, 100)
	assert.NoError(self.T(), err)

	// ListChildren gives the full path to all children
	assert.Equal(self.T(), children, []string{
		"/a/b/c/1",
		"/a/b/c/2",
		"/a/b/c/3"})

	children, err = self.datastore.ListChildren(self.config_obj, urn, 0, 2)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), children, []string{
		"/a/b/c/1", "/a/b/c/2"})

	children, err = self.datastore.ListChildren(self.config_obj, urn, 1, 2)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), children, []string{
		"/a/b/c/2", "/a/b/c/3"})
}

func (self MysqlTestSuite) TestIndexes() {
	client_id := "C.1234"
	client_id_2 := "C.1235"
	err := self.datastore.SetIndex(self.config_obj, constants.CLIENT_INDEX_URN,
		client_id, []string{"all", client_id, "Hostname", "FQDN", "host:Foo"})
	assert.NoError(self.T(), err)
	err = self.datastore.SetIndex(self.config_obj, constants.CLIENT_INDEX_URN,
		client_id_2, []string{"all", client_id_2, "Hostname2", "FQDN2", "host:Bar"})
	assert.NoError(self.T(), err)

	hits := self.datastore.SearchClients(self.config_obj, constants.CLIENT_INDEX_URN,
		"all", "", 0, 100)
	sort.Strings(hits)
	assert.Equal(self.T(), []string{client_id, client_id_2}, hits)

	hits = self.datastore.SearchClients(self.config_obj, constants.CLIENT_INDEX_URN,
		"*foo", "", 0, 100)
	assert.Equal(self.T(), []string{client_id}, hits)

}

func TestMysqlDatabase(t *testing.T) {
	// If a local testing mysql server is configured we can run
	// this test, otherwise skip it.
	config_obj, err := new(config.Loader).WithFileLoader(
		"test_data/mysql.config.yaml").
		LoadAndValidate()
	if err != nil {
		return
	}

	suite.Run(t, &MysqlTestSuite{
		config_obj: config_obj,
	})
}
