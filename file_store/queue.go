package file_store

import (
	"errors"
	"fmt"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/directory"
	"www.velocidex.com/golang/velociraptor/file_store/memory"
	"www.velocidex.com/golang/velociraptor/file_store/mysql"
)

// GetQueueManager selects an appropriate QueueManager object based on
// config.
func GetQueueManager(config_obj *config_proto.Config) (api.QueueManager, error) {
	file_store := GetFileStore(config_obj)

	switch config_obj.Datastore.Implementation {

	// For now everyone uses an in-memory queue manager.
	case "Test":
		return memory.NewMemoryQueueManager(config_obj, file_store), nil

	case "FileBaseDataStore":
		return directory.NewDirectoryQueueManager(config_obj, file_store), nil

	case "MySQL":
		return mysql.NewMysqlQueueManager(file_store.(*mysql.SqlFileStore))

	default:
		return nil, errors.New(fmt.Sprintf("Unsupported QueueManager %v",
			config_obj.Datastore.Implementation))
	}
}
