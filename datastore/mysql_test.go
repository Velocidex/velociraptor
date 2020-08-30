package datastore

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
)

type MysqlTestSuite struct {
	BaseTestSuite
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

func (self *MysqlTestSuite) TearDownTest() {
	if self.datastore != nil {
		self.datastore.Close()
	}
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

	suite.Run(t, &MysqlTestSuite{BaseTestSuite{
		config_obj: config_obj,
	}})
}
