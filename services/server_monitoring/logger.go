package server_monitoring

import (
	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

type serverLogger struct {
	config_obj   *config_proto.Config
	path_manager api.PathManager
	Clock        utils.Clock
}

// Send each log message individually to avoid any buffering - logs
// need to be available immediately.
func (self *serverLogger) Write(b []byte) (int, error) {
	// Path may change as the log files rotate.
	path, err := self.path_manager.GetPathForWriting()
	if err != nil {
		return 0, err
	}

	journal, err := services.GetJournal()
	if err != nil {
		return 0, err
	}

	msg := artifacts.DeobfuscateString(self.config_obj, string(b))
	err = journal.AppendToResultSet(self.config_obj,
		path, []*ordereddict.Dict{
			ordereddict.NewDict().
				Set("_ts", self.Clock.Now().UTC().UnixNano()/1000).
				Set("Timestamp", self.Clock.Now().UTC().String()).
				Set("Message", msg)})
	return len(b), err
}
