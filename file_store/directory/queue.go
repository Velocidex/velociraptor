package directory

import (
	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/memory"
	"www.velocidex.com/golang/velociraptor/file_store/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

var (
	pool = memory.NewQueuePool()
)

type DirectoryQueueManager struct {
	config_obj *config_proto.Config
	FileStore  api.FileStore
	scope      *vfilter.Scope
	Clock      utils.Clock
}

func (self *DirectoryQueueManager) PushEventRows(
	path_manager api.PathManager, dict_rows []*ordereddict.Dict) error {

	for _, row := range dict_rows {
		pool.Broadcast(path_manager.GetQueueName(),
			row.Set("_ts", int(self.Clock.Now().Unix())))
	}

	rs_writer, err := result_sets.NewResultSetWriter(
		self.config_obj, path_manager, nil, false /* truncate */)
	if err != nil {
		return err
	}
	defer rs_writer.Close()

	for _, row := range dict_rows {
		// Set a timestamp per event for easier querying.
		row.Set("_ts", int(self.Clock.Now().Unix()))
		rs_writer.Write(row)
	}
	return nil
}

func (self *DirectoryQueueManager) Watch(
	queue_name string) (output <-chan *ordereddict.Dict, cancel func()) {
	return pool.Register(queue_name)
}

func NewDirectoryQueueManager(config_obj *config_proto.Config,
	file_store api.FileStore) api.QueueManager {
	return &DirectoryQueueManager{
		config_obj: config_obj,
		FileStore:  file_store,
		scope:      vfilter.NewScope(),
		Clock:      utils.RealClock{},
	}
}
