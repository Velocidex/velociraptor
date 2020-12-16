package mysql

import (
	"database/sql"
	"fmt"

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

	db.Exec(fmt.Sprintf("drop database if exists `%v`", database))

	config_obj.Datastore.MysqlConnectionString = conn_string + database
	initializeDatabase(config_obj, conn_string+database, database)

	return NewSqlFileStore(config_obj)
}
