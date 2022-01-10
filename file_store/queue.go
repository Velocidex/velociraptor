package file_store

import (
	"errors"
	"fmt"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/directory"
	"www.velocidex.com/golang/velociraptor/file_store/memory"
)

// GetQueueManager selects an appropriate QueueManager object based on
// config.
func GetQueueManager(config_obj *config_proto.Config) (api.QueueManager, error) {
	if config_obj.Datastore == nil {
		return nil, errors.New("Datastore not configured")
	}

	file_store := GetFileStore(config_obj)
	implementation, err := datastore.GetImplementationName(config_obj)
	if err != nil {
		return nil, err
	}

	switch implementation {

	// For now everyone uses an in-memory queue manager.
	case "Test":
		return memory.NewMemoryQueueManager(config_obj, file_store), nil

	case "FileBaseDataStore", "MemcacheFileDataStore",
		"RemoteFileDataStore", "ReadOnlyDataStore":
		return directory.NewDirectoryQueueManager(config_obj, file_store), nil

	default:
		return nil, fmt.Errorf("Unsupported QueueManager %v",
			config_obj.Datastore.Implementation)
	}
}
