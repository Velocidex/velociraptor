package memory_test

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/memory"

	_ "www.velocidex.com/golang/velociraptor/file_store/result_sets/simple"
	_ "www.velocidex.com/golang/velociraptor/file_store/result_sets/timed"
)

func TestMemoryQueueManager(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	manager := memory.NewMemoryQueueManager(config_obj, memory.Test_memory_file_store)
	suite.Run(t, api.NewQueueManagerTestSuite(config_obj, manager, memory.Test_memory_file_store))
}
