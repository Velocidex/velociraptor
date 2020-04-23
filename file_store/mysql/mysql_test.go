package mysql

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

func SetupTest(config_obj *config_proto.Config) (api.FileStore, error) {
	// Drop and initialize the database to start a new test.
	conn_string := fmt.Sprintf("%s:%s@tcp(%s)/",
		config_obj.Datastore.MysqlUsername,
		config_obj.Datastore.MysqlPassword,
		config_obj.Datastore.MysqlServer)

	// Make sure our database is not the same as the datastore
	// tests or else we will trash over them.
	config_obj.Datastore.MysqlDatabase = "velociraptor_testfs"
	database := config_obj.Datastore.MysqlDatabase

	db, err := sql.Open("mysql", conn_string)
	if err != nil {
		return nil, err
	}

	defer db.Close()

	err = db.Ping()
	if err != nil {
		return nil, err
	}

	db.Exec(fmt.Sprintf("drop database `%v`", database))

	config_obj.Datastore.MysqlConnectionString = conn_string + database
	initializeDatabase(config_obj, conn_string+database, database)

	return NewSqlFileStore(config_obj)
}

func TestMysqlDatabase(t *testing.T) {
	// If a local testing mysql server is configured we can run
	// this test, otherwise skip it.
	config_obj, err := config.LoadConfig("../datastore/test_data/mysql.config.yaml")
	if err != nil {
		return
	}

	file_store, err := SetupTest(config_obj)
	if err != nil {
		t.Skipf("Unable to contact mysql - skipping: %v", err)
		return
	}

	suite.Run(t, api.NewFileStoreTestSuite(config_obj, file_store))
}
