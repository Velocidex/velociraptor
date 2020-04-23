package file_store

import (
	"fmt"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/memory"
)

// GetQueueManager selects an appropriate QueueManager object based on
// config.
func GetQueueManager(config_obj *config_proto.Config) api.QueueManager {
	file_store := GetFileStore(config_obj)

	switch config_obj.Datastore.Implementation {

	// For now everyone uses an in-memory queue manager.
	case "FileBaseDataStore", "MySQL", "Test":
		return memory.NewMemoryQueueManager(config_obj, file_store)

	default:
		panic(fmt.Sprintf("Unsupported QueueManager %v",
			config_obj.Datastore.Implementation))
	}
}
