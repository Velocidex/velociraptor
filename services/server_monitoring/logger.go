package server_monitoring

import (
	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/result_sets/timed"
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
	file_store_factory := file_store.GetFileStore(self.config_obj)

	writer, err := timed.NewTimedResultSetWriterWithClock(
		file_store_factory, self.path_manager, nil,
		api.SyncCompleter, self.Clock)
	if err != nil {
		return 0, err
	}
	defer writer.Close()

	// Logs for event queries are written to timed result sets just
	// like the regular artifacts.
	msg := artifacts.DeobfuscateString(self.config_obj, string(b))
	writer.Write(ordereddict.NewDict().
		Set("Timestamp", self.Clock.Now().UTC().String()).
		Set("Message", msg))

	return len(b), nil
}
