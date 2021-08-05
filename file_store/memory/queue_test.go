package memory_test

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/file_store/memory"
	"www.velocidex.com/golang/velociraptor/file_store/tests"

	_ "www.velocidex.com/golang/velociraptor/result_sets/simple"
	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

func TestMemoryQueueManager(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	file_store := memory.NewMemoryFileStore(config_obj)
	manager := memory.NewMemoryQueueManager(config_obj, file_store)
	suite.Run(t, tests.NewQueueManagerTestSuite(config_obj, manager, file_store))
}
