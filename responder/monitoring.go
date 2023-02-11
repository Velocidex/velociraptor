package responder

import (
	"context"
	"sync"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

type MonitoringManager struct {
	mu sync.Mutex

	// Key is artifact name
	in_flight map[string]*MonitoringContext
}

// Flush all the monitoring messages at the same time so they are
// queued in the same packet.
func (self *MonitoringManager) flushLogMessages(ctx context.Context) {
	self.mu.Lock()
	snapshot := make([]*MonitoringContext, 0, len(self.in_flight))
	for _, v := range self.in_flight {
		snapshot = append(snapshot, v)
	}
	self.mu.Unlock()

	for _, c := range snapshot {
		c.flushLogMessages(ctx)
	}
}

func (self *MonitoringManager) Context(
	output chan *crypto_proto.VeloMessage, artifact string) *MonitoringContext {
	self.mu.Lock()
	defer self.mu.Unlock()

	monitoring_context, pres := self.in_flight[artifact]
	if !pres {
		monitoring_context := &MonitoringContext{
			output:   output,
			artifact: artifact,
		}
		self.in_flight[artifact] = monitoring_context
		return monitoring_context
	}

	return monitoring_context
}

func NewMonitoringManager(ctx context.Context) *MonitoringManager {
	result := &MonitoringManager{
		in_flight: make(map[string]*MonitoringContext),
	}

	batch_delay := uint64(5)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return

			case <-time.After(time.Second * time.Duration(batch_delay)):
				result.flushLogMessages(ctx)
			}
		}
	}()

	return result
}

type MonitoringContext struct {
	mu     sync.Mutex
	output chan *crypto_proto.VeloMessage

	log_messages      []byte
	log_messages_id   uint64 // The ID of the first row in the log_messages buffer
	log_message_count uint64
	artifact          string

	upload_id int64
}

func (self *MonitoringContext) NextUploadId() int64 {
	self.mu.Lock()
	defer self.mu.Unlock()
	result := self.upload_id
	self.upload_id++
	return result
}

func (self *MonitoringContext) AddLogMessage(level string, msg string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.log_message_count++
	self.log_messages = append(self.log_messages, json.Format(
		"{\"client_time\":%d,\"level\":%q,\"message\":%q}\n",
		int(utils.GetTime().Now().Unix()), level, msg)...)
}

func (self *MonitoringContext) getLogMessages() (
	buf []byte, start_id uint64, message_count uint64) {
	buf = self.log_messages
	message_count = self.log_message_count
	start_id = self.log_messages_id

	self.log_messages = nil
	self.log_message_count = 0
	self.log_messages_id = start_id + message_count

	return buf, start_id, message_count
}

func (self *MonitoringContext) flushLogMessages(ctx context.Context) {
	buf, id, count := self.getLogMessages()
	if len(buf) > 0 {
		message := &crypto_proto.VeloMessage{
			SessionId: "F.Monitoring",
			RequestId: constants.LOG_SINK,
			LogMessage: &crypto_proto.LogMessage{
				Id:           int64(id),
				NumberOfRows: count,
				Jsonl:        string(buf),
				Artifact:     self.artifact,
			}}

		select {
		case <-ctx.Done():
			return

		case self.output <- message:
		}
	}
}

// A responder for client monitoring queries. Since monitoring queries
// do not really have state we provide a very simple responder.
type MonitoringResponder struct {
	ctx context.Context

	// Where to send the output
	output chan *crypto_proto.VeloMessage

	// Name of the artifact
	artifact string

	monitoring_context *MonitoringContext
	config_obj         *config_proto.Config
}

func NewMonitoringResponder(
	ctx context.Context,
	config_obj *config_proto.Config,
	monitoring_manager *MonitoringManager,
	output chan *crypto_proto.VeloMessage,
	artifact string) *MonitoringResponder {

	return &MonitoringResponder{
		ctx:                ctx,
		output:             output,
		artifact:           artifact,
		monitoring_context: monitoring_manager.Context(output, artifact),
		config_obj:         config_obj,
	}
}

func (self *MonitoringResponder) AddResponse(message *crypto_proto.VeloMessage) {
	message.SessionId = "F.Monitoring"

	select {
	case <-self.ctx.Done():
		break

	case self.output <- message:
	}
}

// Monitoring queries dont have a status - the logs will be of type error.
func (self *MonitoringResponder) RaiseError(ctx context.Context, message string) {
	logger := logging.GetLogger(self.config_obj, &logging.ClientComponent)
	logger.Error("MonitoringResponder: %v", message)
}

func (self *MonitoringResponder) Return(ctx context.Context) {}

// Logs will be batched.
func (self *MonitoringResponder) Log(ctx context.Context, level string, msg string) {
	self.monitoring_context.AddLogMessage(level, msg)
}

func (self *MonitoringResponder) NextUploadId() int64 {
	return self.monitoring_context.NextUploadId()
}

func (self *MonitoringResponder) Close() {}
