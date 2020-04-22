package file_store

import (
	"fmt"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/directory"
)

// GetQueueManager selects an appropriate QueueManager object based on
// config.
func GetQueueManager(config_obj *config_proto.Config) api.QueueManager {
	switch config_obj.Datastore.Implementation {
	case "FileBaseDataStore":
		return directory.NewDirectoryQueueManager(config_obj)

	default:
		panic(fmt.Sprintf("Unsupported QueueManager %v",
			config_obj.Datastore.Implementation))
	}
}
