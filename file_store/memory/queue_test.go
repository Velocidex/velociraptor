package memory

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

func TestMemoryQueueManager(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	manager := NewMemoryQueueManager(config_obj, Test_memory_file_store)
	suite.Run(t, api.NewQueueManagerTestSuite(config_obj, manager, Test_memory_file_store))
}
