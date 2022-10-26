package reporting

import (
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
)

type notebooCellLogger struct {
	mu       sync.Mutex
	messages []string

	rs_writer result_sets.ResultSetWriter

	// If true we have more messages in the result set.
	more_messages bool
}

func newNotebookCellLogger(
	config_obj *config_proto.Config, log_path api.FSPathSpec) (
	*notebooCellLogger, error) {
	file_store_factory := file_store.GetFileStore(config_obj)

	// Create a new result set to write the logs
	rs_writer, err := result_sets.NewResultSetWriter(
		file_store_factory, log_path, json.NewEncOpts(),
		utils.BackgroundWriter, result_sets.TruncateMode)
	if err != nil {
		return nil, err
	}

	return &notebooCellLogger{
		rs_writer: rs_writer,
	}, nil
}

func (self *notebooCellLogger) Write(b []byte) (int, error) {
	level, msg := logging.SplitIntoLevelAndLog(b)
	self.rs_writer.Write(ordereddict.NewDict().
		Set("Timestamp", time.Now().UTC().UnixNano()/1000).
		Set("Level", level).
		Set("message", msg))

	// Only keep the first 10 messages in the cell. This provides a
	// good balance between seeing if the query worked and examining
	// all the log messages.
	self.mu.Lock()
	if len(self.messages) < 10 {
		self.messages = append(self.messages, msg)
	} else {
		self.more_messages = true
	}
	self.mu.Unlock()

	return len(b), nil
}

func (self *notebooCellLogger) Messages() []string {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.messages
}

func (self *notebooCellLogger) MoreMessages() bool {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.more_messages
}

func (self *notebooCellLogger) Flush() {
	self.rs_writer.Flush()
}
