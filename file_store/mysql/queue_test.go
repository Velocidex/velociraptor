package mysql

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

func TestMysqlQueueManager(t *testing.T) {
	config_obj, err := config.LoadConfig(
		"../../datastore/test_data/mysql.config.yaml")
	if err != nil {
		return
	}

	file_store, err := SetupTest(config_obj)
	if err != nil {
		t.Skipf("Unable to contact mysql - skipping: %v", err)
		return
	}

	manager := NewMysqlQueueManager(file_store.(*SqlFileStore))
	suite.Run(t, api.NewQueueManagerTestSuite(config_obj, manager, file_store))
}
