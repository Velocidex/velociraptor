package directory

import (
	"fmt"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/memory"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

var (
	pool = memory.NewQueuePool()
)

type DirectoryQueueManager struct {
	FileStore api.FileStore
	scope     *vfilter.Scope
	Clock     utils.Clock
}

func (self *DirectoryQueueManager) PushEventRows(
	path_manager api.PathManager, dict_rows []*ordereddict.Dict) error {

	for _, row := range dict_rows {
		pool.Broadcast(path_manager.GetQueueName(),
			row.Set("_ts", int(self.Clock.Now().Unix())))
	}

	log_path, err := path_manager.GetPathForWriting()
	if err != nil {
		return err
	}

	fd, err := self.FileStore.WriteFile(log_path)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return err
	}
	defer fd.Close()

	for _, row := range dict_rows {
		// Set a timestamp per event for easier querying.
		row.Set("_ts", int(self.Clock.Now().Unix()))
	}

	serialized, err := utils.DictsToJson(dict_rows)
	if err != nil {
		return err
	}

	_, err = fd.Write(serialized)
	return err
}

func (self *DirectoryQueueManager) Watch(
	queue_name string) (output <-chan *ordereddict.Dict, cancel func()) {
	return pool.Register(queue_name)
}

func NewDirectoryQueueManager(config_obj *config_proto.Config,
	file_store api.FileStore) api.QueueManager {
	return &DirectoryQueueManager{
		FileStore: file_store,
		scope:     vfilter.NewScope(),
		Clock:     utils.RealClock{},
	}
}
