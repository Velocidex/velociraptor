package server_artifacts

import (
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
)

type serverLogger struct {
	collection_context CollectionContextManager
	config_obj         *config_proto.Config
	writer             result_sets.ResultSetWriter
}

func (self *serverLogger) Close() {
	self.writer.Close()
}

func (self *serverLogger) Write(b []byte) (int, error) {
	level, msg := logging.SplitIntoLevelAndLog(b)
	msg = artifacts.DeobfuscateString(self.config_obj, msg)

	self.writer.Write(ordereddict.NewDict().
		Set("Timestamp", time.Now().UTC().UnixNano()/1000).
		Set("Level", level).
		Set("message", msg))

	// Increment the log count.
	self.collection_context.Modify(func(context *flows_proto.ArtifactCollectorContext) {
		context.TotalLogs++
	})

	return len(b), nil
}

func NewServerLogger(
	collection_context CollectionContextManager,
	config_obj *config_proto.Config,
	session_id string) (*serverLogger, error) {

	path_manager := paths.NewFlowPathManager("server", session_id)
	file_store_factory := file_store.GetFileStore(config_obj)
	writer, err := result_sets.NewResultSetWriter(file_store_factory,
		path_manager.Log(), json.NoEncOpts,
		utils.BackgroundWriter, result_sets.AppendMode)
	if err != nil {
		return nil, err
	}

	return &serverLogger{
		collection_context: collection_context,
		config_obj:         config_obj,
		writer:             writer,
	}, nil
}
