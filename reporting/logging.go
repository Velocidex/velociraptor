package reporting

import (
	"context"
	"sync"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

type notebookCellLogger struct {
	mu       sync.Mutex
	messages []string

	rs_writer result_sets.ResultSetWriter

	// If true we have more messages in the result set.
	more_messages bool

	ctx        context.Context
	config_obj *config_proto.Config
}

func newNotebookCellLogger(
	ctx context.Context,
	config_obj *config_proto.Config, log_path api.FSPathSpec) (
	*notebookCellLogger, error) {
	file_store_factory := file_store.GetFileStore(config_obj)

	// Create a new result set to write the logs
	rs_writer, err := result_sets.NewResultSetWriter(
		file_store_factory, log_path, json.NewEncOpts(),
		utils.BackgroundWriter, result_sets.TruncateMode)
	if err != nil {
		return nil, err
	}

	return &notebookCellLogger{
		ctx:        ctx,
		config_obj: config_obj,
		rs_writer:  rs_writer,
	}, nil
}

func (self *notebookCellLogger) Write(b []byte) (int, error) {
	level, msg := logging.SplitIntoLevelAndLog(b)
	self.rs_writer.Write(ordereddict.NewDict().
		Set("Timestamp", utils.GetTime().Now().UTC().UnixNano()/1000).
		Set("Level", level).
		Set("message", msg))

	if level == logging.ALERT {
		err := self.processAlert(msg)
		if err != nil {
			return 0, err
		}
	}

	// Only keep the first 10 messages in the cell. This provides a
	// good balance between seeing if the query worked and examining
	// all the log messages.
	self.mu.Lock()
	if len(self.messages) < 10 {
		snippet := string(b)
		if len(snippet) > 1000 {
			snippet = snippet[:100] + " ..."
		}
		self.messages = append(self.messages, snippet)

	} else {
		self.more_messages = true

		// Make sure errors are always shown in the snippet even if
		// they get pushed out by earlier messages
		if level == "ERROR" {
			self.messages = append(self.messages, string(b))
			self.messages = self.messages[1:]
		}
	}
	self.mu.Unlock()

	return len(b), nil
}

func (self *notebookCellLogger) processAlert(msg string) error {
	alert := &services.AlertMessage{}
	err := json.Unmarshal([]byte(msg), alert)
	if err != nil {
		return err
	}

	alert.ClientId = "server"
	serialized, err := json.Marshal(alert)
	if err != nil {
		return err
	}
	serialized = append(serialized, '\n')

	journal, err := services.GetJournal(self.config_obj)
	if err != nil {
		return err
	}
	return journal.PushJsonlToArtifact(self.ctx, self.config_obj,
		serialized, 1, "Server.Internal.Alerts", "server", "")
}

func (self *notebookCellLogger) Messages() []string {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.messages
}

// Are there additional messages?
func (self *notebookCellLogger) MoreMessages() bool {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.more_messages
}

func (self *notebookCellLogger) Flush() {
	self.rs_writer.Flush()
}

func (self *notebookCellLogger) Close() {
	self.rs_writer.Close()
}
