package flows

import (
	"fmt"
	"regexp"
	"sync"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	utils "www.velocidex.com/golang/velociraptor/utils"
)

var (
	defaultLogErrorRegex = regexp.MustCompile(constants.VQL_ERROR_REGEX)

	// If the config file specifies the regex we compile it once and
	// cache in memory.
	mu            sync.Mutex
	logErrorRegex *regexp.Regexp
)

func getLogErrorRegex(config_obj *config_proto.Config) *regexp.Regexp {
	if config_obj.Frontend.CollectionErrorRegex != "" {
		mu.Lock()
		defer mu.Unlock()

		if logErrorRegex == nil {
			logErrorRegex = regexp.MustCompile(
				config_obj.Frontend.CollectionErrorRegex)
		}
		return logErrorRegex
	}

	return defaultLogErrorRegex
}

// Flush the logs to disk. During execution the flow collects the logs
// in memory and then flushes it all when done.
func flushContextLogs(
	config_obj *config_proto.Config,
	collection_context *CollectionContext,
	completion *utils.Completer) error {

	// Handle monitoring flow specially.
	if collection_context.SessionId == constants.MONITORING_WELL_KNOWN_FLOW {
		return flushContextLogsMonitoring(config_obj, collection_context)
	}

	flow_path_manager := paths.NewFlowPathManager(
		collection_context.ClientId,
		collection_context.SessionId).Log()

	// Append logs to messages from previous packets.
	file_store_factory := file_store.GetFileStore(config_obj)
	rs_writer, err := result_sets.NewResultSetWriter(
		file_store_factory, flow_path_manager,
		json.NoEncOpts, completion.GetCompletionFunc(),
		false /* truncate */)
	if err != nil {
		return err
	}
	defer rs_writer.Close()

	for _, row := range collection_context.Logs {
		// If the log message matches the error regex mark the
		// collection as errored out. Only record the first error.
		if collection_context.State != flows_proto.
			ArtifactCollectorContext_ERROR {

			// If any messages are of level ERROR or the message
			// matches the regex, then the collection is considered
			// errored.
			if row.Level == logging.ERROR ||
				getLogErrorRegex(config_obj).FindStringIndex(row.Message) != nil {
				collection_context.State = flows_proto.ArtifactCollectorContext_ERROR
				collection_context.Status = row.Message
				collection_context.Dirty = true
			}
		}

		collection_context.TotalLogs++
		rs_writer.WriteJSONL([]byte(fmt.Sprintf(
			"{\"_ts\":%d,\"client_time\":%d,\"level\":%q,\"message\":%q}\n",
			int(time.Now().Unix()),
			int64(row.Timestamp)/1000000,
			row.Level,
			row.Message)), 1)
	}

	// Clear the logs from the flow object.
	collection_context.Logs = nil
	return nil
}
