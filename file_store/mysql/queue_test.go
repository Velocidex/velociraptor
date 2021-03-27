// +build deprecated

package mysql

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

func TestMysqlQueueManager(t *testing.T) {
	config_obj, err := new(config.Loader).WithFileLoader(
		"../../datastore/test_data/mysql.config.yaml").
		LoadAndValidate()
	require.NoError(t, err)

	file_store, err := SetupTest(config_obj)
	if err != nil {
		t.Skipf("Unable to contact mysql - skipping: %v", err)
		return
	}

	manager := NewMysqlQueueManager(file_store.(*SqlFileStore))
	suite.Run(t, api.NewQueueManagerTestSuite(config_obj, manager, file_store))
}
