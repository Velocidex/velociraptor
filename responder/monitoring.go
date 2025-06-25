package responder

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services/debug"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type MonitoringManager struct {
	mu sync.Mutex

	// Key is artifact name
	in_flight map[string]*MonitoringContext
}

func (self *MonitoringManager) reap() {
	now := utils.GetTime().Now()

	for k, v := range self.in_flight {
		if !v.finished.IsZero() && now.Sub(v.finished) > time.Minute*5 {
			delete(self.in_flight, k)
		}
	}
}

func (self *MonitoringManager) WriteProfile(
	ctx context.Context, scope vfilter.Scope,
	output_chan chan vfilter.Row) {

	self.mu.Lock()

	self.reap()

	var contexts []*MonitoringContext
	for _, v := range self.in_flight {
		contexts = append(contexts, v)
	}
	sort.Slice(contexts, func(i, j int) bool {
		return contexts[i].id < contexts[j].id
	})

	self.mu.Unlock()

	for _, v := range contexts {
		output_chan <- v.Stats()
	}
}

// Flush all the monitoring messages at the same time so they are
// queued in the same packet.
func (self *MonitoringManager) flushLogMessages(ctx context.Context) {
	self.mu.Lock()

	self.reap()

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
	output chan *crypto_proto.VeloMessage, artifact string, id int64) *MonitoringContext {

	self.mu.Lock()
	defer self.mu.Unlock()

	self.reap()

	key := fmt.Sprintf("%s-%v", artifact, utils.GetId())
	monitoring_context, pres := self.in_flight[key]
	if !pres {
		monitoring_context := &MonitoringContext{
			id:       id,
			output:   output,
			artifact: artifact,
			started:  utils.GetTime().Now(),
		}
		self.in_flight[key] = monitoring_context
		return monitoring_context
	}

	return monitoring_context
}

func NewMonitoringManager(ctx context.Context) *MonitoringManager {
	result := &MonitoringManager{
		in_flight: make(map[string]*MonitoringContext),
	}

	batch_delay := uint64(5)

	info := debug.ProfileWriterInfo{
		Name:          "Client Monitoring Manager",
		Description:   "Report stats on client monitoring artifacts",
		ProfileWriter: result.WriteProfile,
		ID:            utils.GetId(),
		Categories:    []string{"Client"},
	}

	debug.RegisterProfileWriter(info)

	go func() {
		defer debug.UnregisterProfileWriter(info.ID)

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

	id int64

	log_messages      []byte
	log_messages_id   uint64 // The ID of the first row in the log_messages buffer
	log_message_count uint64
	artifact          string

	upload_id int64

	started, finished time.Time
	sent_rows         uint64
	bytes             uint64
}

func (self *MonitoringContext) Stats() *ordereddict.Dict {
	self.mu.Lock()
	defer self.mu.Unlock()

	duration := "running"
	if !self.finished.IsZero() {
		duration = self.finished.Sub(self.started).Round(time.Second).String()
	}

	return ordereddict.NewDict().
		Set("Artifact", self.artifact).
		Set("QueryId", self.id).
		Set("Started", self.started).
		Set("StartedAgo", utils.GetTime().Now().Sub(
			self.started).Round(time.Second).String()).
		Set("Duration", duration).
		Set("Logs", self.log_messages_id+self.log_message_count).
		Set("Rows", self.sent_rows).
		Set("JSONBytes", self.bytes)
}

func (self *MonitoringContext) ChargeResponses(response *actions_proto.VQLResponse) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.sent_rows += response.TotalRows
	self.bytes += uint64(len(response.JSONLResponse))
}

func (self *MonitoringContext) NextUploadId() int64 {
	self.mu.Lock()
	defer self.mu.Unlock()
	result := self.upload_id
	self.upload_id++
	return result
}

// Alert messages are sent in their own packet because the server will
// redirect them into the alert queue.
func (self *MonitoringContext) sendAlertMessage(
	ctx context.Context, level string,

	// msg containes serialized services.AlertMessage
	msg string) {

	self.mu.Lock()
	id := self.log_messages_id
	self.log_messages_id++
	self.mu.Unlock()

	self.output <- &crypto_proto.VeloMessage{
		SessionId: "F.Monitoring",
		RequestId: constants.LOG_SINK,
		LogMessage: &crypto_proto.LogMessage{
			Id:           int64(id),
			NumberOfRows: 1,
			Jsonl: json.Format(
				"{\"client_time\":%d,\"level\":%q,\"message\":%q}\n",
				int(utils.GetTime().Now().Unix()), level, msg),
			Level:    logging.ALERT,
			Artifact: self.artifact,
		}}
}

func (self *MonitoringContext) AddLogMessage(
	ctx context.Context, level string, msg string) {
	if level == logging.ALERT {
		self.sendAlertMessage(ctx, level, msg)
		return
	}

	self.mu.Lock()
	defer self.mu.Unlock()

	message := json.Format("{\"client_time\":%d,\"level\":%q,\"message\":%q}\n",
		int(utils.GetTime().Now().Unix()), level, msg)
	self.log_message_count++
	self.log_messages = append(self.log_messages, message...)

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

func (self *MonitoringContext) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.finished = utils.GetTime().Now()
}

func (self *MonitoringContext) flushLogMessages(ctx context.Context) {
	self.mu.Lock()
	buf, id, count := self.getLogMessages()
	if len(buf) == 0 {
		self.mu.Unlock()
		return
	}

	// Include the logs in the bytes sent figure.
	self.bytes += uint64(len(buf))

	// Do not block with lock held
	self.mu.Unlock()

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
	artifact string,
	query_id int64) *MonitoringResponder {

	return &MonitoringResponder{
		ctx:                ctx,
		output:             output,
		artifact:           artifact,
		monitoring_context: monitoring_manager.Context(output, artifact, query_id),
		config_obj:         config_obj,
	}
}

func (self *MonitoringResponder) FlowContext() *FlowContext {
	return &FlowContext{
		flow_id: "F.Monitoring",
	}
}

func (self *MonitoringResponder) AddResponse(message *crypto_proto.VeloMessage) {
	message.SessionId = "F.Monitoring"

	if message.VQLResponse != nil {
		self.monitoring_context.ChargeResponses(message.VQLResponse)
	}

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
	self.monitoring_context.AddLogMessage(ctx, level, msg)
}

func (self *MonitoringResponder) NextUploadId() int64 {
	return self.monitoring_context.NextUploadId()
}

func (self *MonitoringResponder) Close() {
	self.monitoring_context.Close()
}
