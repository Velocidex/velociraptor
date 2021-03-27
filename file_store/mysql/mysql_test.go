// +build deprecated

package mysql

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

func TestMysqlDatabase(t *testing.T) {
	// If a local testing mysql server is configured we can run
	// this test, otherwise skip it.
	config_obj, err := new(config.Loader).WithFileLoader(
		"../../datastore/test_data/mysql.config.yaml").
		LoadAndValidate()
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
