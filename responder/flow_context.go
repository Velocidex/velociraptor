package responder

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"sync"
	"sync/atomic"
	"time"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

// Represents a single flow on the client. Previously flows were
// tracked on the server but now they are completely tracked on the
// client, and simply synced to the server. This dramatically reduces
// the amount of work done on the server.
type FlowContext struct {
	ctx        context.Context
	config_obj *config_proto.Config
	flow_id    string

	// The original request.
	req *crypto_proto.FlowRequest

	// Flow wide totals
	total_rows           uint64
	total_uploaded_bytes uint64

	// Send the messages to this channel
	output chan *crypto_proto.VeloMessage

	// The path to the checkpoint file.
	checkpoint string

	// Cancelling the FlowContext will cancel all its in flight
	// queries.
	cancel func()

	// A single flow can have multiple queries, each is tracked by its
	// own responder. We track the life of each responder with this wg.
	wg         *sync.WaitGroup
	responders []*FlowResponder

	// Logs and uploads are managed per collection, and are shared
	// with all the queries.

	// A counter of uploads sent in the entire collection.
	upload_id int32

	// A JSONL buffer with log messages collected for the entire flow.
	mu                sync.Mutex
	log_messages      []byte
	log_messages_id   uint64 // The ID of the first row in the log_messages buffer
	log_message_count uint64
	error_message     string // If an error occurs trap the error message

	last_stats_timestamp uint64
	frequency_msec       uint64

	// We ensure to only send the final flow complete message
	// once. This will trigger a System.Flow.Completion event on the
	// server.
	final_stats_sent bool

	// from the flow manager when the FlowContext is complete.
	owner *FlowManager
}

func newFlowContext(ctx context.Context,
	config_obj *config_proto.Config,
	output chan *crypto_proto.VeloMessage,
	req *crypto_proto.VeloMessage, owner *FlowManager) *FlowContext {

	if req.FlowRequest == nil {
		req.FlowRequest = &crypto_proto.FlowRequest{}
	}

	flow_id := req.SessionId

	// How often do we send a FlowStat message to sync the server's
	// flow progress stat.
	frequency_msec := uint64(5000)
	if config_obj != nil &&
		config_obj.Client != nil &&
		config_obj.Client.DefaultServerFlowStatsUpdate > 0 {
		frequency_msec = config_obj.Client.DefaultServerFlowStatsUpdate
	}
	if req.FlowRequest.FlowUpdateTime > 0 {
		frequency_msec = req.FlowRequest.FlowUpdateTime
	}

	// Default is set by config file
	batch_delay := uint64(5000)
	if config_obj != nil &&
		config_obj.Frontend != nil &&
		config_obj.Frontend.Resources != nil &&
		config_obj.Frontend.Resources.DefaultLogBatchTime > 0 {
		batch_delay = config_obj.Frontend.Resources.DefaultLogBatchTime
	}
	if req.FlowRequest.LogBatchTime > 0 {
		batch_delay = req.FlowRequest.LogBatchTime
	}

	// Allow the flow to be cancelled.
	sub_ctx, cancel := context.WithCancel(ctx)
	self := &FlowContext{
		ctx:            sub_ctx,
		cancel:         cancel,
		wg:             &sync.WaitGroup{},
		output:         output,
		req:            req.FlowRequest,
		frequency_msec: frequency_msec,
		config_obj:     config_obj,
		flow_id:        flow_id,
		owner:          owner,
		checkpoint:     makeCheckpoint(config_obj, flow_id),
	}

	go func() {
		for {
			select {
			case <-sub_ctx.Done():
				return

			case <-time.After(time.Duration(batch_delay) * time.Millisecond):
				stats := self.MaybeSendStats()
				if stats != nil {
					select {
					case <-sub_ctx.Done():
					case self.output <- stats:
					}
				}

				self.FlushLogMessages(ctx)
			}
		}
	}()

	return self
}

func makeCheckpoint(
	config_obj *config_proto.Config,
	flow_id string) string {

	if config_obj == nil ||
		config_obj.Client == nil ||
		config_obj.Client.DisableCheckpoints {
		return ""
	}

	checkpoint, err := ioutil.TempFile("",
		fmt.Sprintf("checkpoint_*.%s", flow_id))
	if err != nil {
		return ""
	}
	// Start off with something sensible.
	checkpoint.Write([]byte(
		json.Format(`{"session_id": %q, "flow_stats": {}}`, flow_id)))
	// We just need the name
	checkpoint.Close()

	config.MutateWriteback(config_obj.Client,
		func(wb *config_proto.Writeback) error {
			wb.Checkpoints = append(wb.Checkpoints,
				&config_proto.FlowCheckPoint{
					FlowId: flow_id,
					Path:   checkpoint.Name(),
				})
			return nil
		})

	logger := logging.GetLogger(config_obj, &logging.ClientComponent)
	logger.Info("Creating a flow checkpoint at <green>%v</>", checkpoint.Name())

	return checkpoint.Name()
}

// Is the flow complete? A flow is complete when all its queries are
// either in the OK or GENERIC_ERROR state.
func (self *FlowContext) IsFlowComplete() bool {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.isFlowComplete()
}

func (self *FlowContext) isFlowComplete() bool {
	// If we have no responders then we have not started executing
	// anything yet.
	if len(self.responders) == 0 {
		return false
	}

	for _, r := range self.responders {
		if !r.IsComplete() {
			return false
		}
	}
	return true
}

func (self *FlowContext) ChargeRows(rows uint64) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.total_rows += rows
	if self.req.MaxRows > 0 && self.total_rows > self.req.MaxRows {
		msg := fmt.Sprintf("Rows %v exceeded limit %v for flow %v. Cancelling.",
			self.total_rows, self.req.MaxRows, self.flow_id)
		return errors.New(msg)
	}
	return nil
}

func (self *FlowContext) ChargeBytes(bytes uint64) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.total_uploaded_bytes += bytes
	if self.req.MaxUploadBytes > 0 &&
		self.total_uploaded_bytes > self.req.MaxUploadBytes {
		msg := fmt.Sprintf("Upload bytes %v exceeded limit %v for flow %v. Cancelling.",
			self.total_uploaded_bytes, self.req.MaxUploadBytes, self.flow_id)
		return errors.New(msg)
	}
	return nil
}

// Cancel all the responders and wait for them to complete. This may
// be called multiple times, but there will be only one log message.
func (self *FlowContext) Cancel() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self._Cancel()
}

func (self *FlowContext) _Cancel() {
	// Cancel all outstanding queries
	for _, r := range self.responders {
		r.RaiseError(self.ctx, "Cancelled")
	}

	self.addLogMessage("ERROR",
		fmt.Sprintf("Cancelled all inflight queries for flow %v", self.flow_id))

	self._Close()
}

func (self *FlowContext) Close() {
	self.mu.Lock()
	self._Close()
	self.mu.Unlock()

	// Wait here until everything is finished. Must be out of the lock
	// to allow the FlowContext to complete.
	self.wg.Wait()
}

func (self *FlowContext) _Close() {
	if self.owner != nil {
		self.owner.removeFlowContext(self.flow_id)
	}
	if self.checkpoint != "" {
		os.Remove(self.checkpoint)
		config.MutateWriteback(self.config_obj.Client,
			func(wb *config_proto.Writeback) error {
				new_list := make([]*config_proto.FlowCheckPoint, 0, len(wb.Checkpoints))
				for _, cp := range wb.Checkpoints {
					if cp.Path != self.checkpoint {
						new_list = append(new_list, cp)
					}
				}

				wb.Checkpoints = new_list
				return nil
			})

		// Do not write a checkpoint any more.
		self.checkpoint = ""
	}

	self.flushLogMessages(self.ctx)
	self.sendStats()
	self.cancel()
}

func (self *FlowContext) SessionId() string {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.flow_id
}

// Drains the error message buffer for transmission
func (self *FlowContext) getLogMessages() (
	buf []byte, start_id uint64, message_count uint64, error_message string) {

	buf = self.log_messages
	message_count = self.log_message_count
	start_id = self.log_messages_id
	error_message = self.error_message

	self.log_messages = nil
	self.log_message_count = 0
	self.log_messages_id = start_id + message_count
	self.error_message = ""

	return buf, start_id, message_count, error_message
}

// Combine cached log messages and send in one message.
func (self *FlowContext) FlushLogMessages(ctx context.Context) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.flushLogMessages(ctx)
}

func (self *FlowContext) flushLogMessages(ctx context.Context) {
	buf, id, count, error_message := self.getLogMessages()
	if len(buf) > 0 {
		self.output <- &crypto_proto.VeloMessage{
			SessionId: self.flow_id,
			RequestId: constants.LOG_SINK,
			LogMessage: &crypto_proto.LogMessage{
				Id:           int64(id),
				NumberOfRows: count,
				Jsonl:        string(buf),
				ErrorMessage: error_message,
			}}
	}
}

// Alert messages are sent in their own packet because the server will
// redirect them into the alert queue.
func (self *FlowContext) sendAlertMessage(
	ctx context.Context, level string,
	// msg containes serialized services.AlertMessage
	msg string) {

	self.mu.Lock()
	id := self.log_messages_id
	self.log_messages_id++
	self.mu.Unlock()

	self.output <- &crypto_proto.VeloMessage{
		SessionId: self.flow_id,
		RequestId: constants.LOG_SINK,
		LogMessage: &crypto_proto.LogMessage{
			Id:           int64(id),
			NumberOfRows: 1,
			Jsonl: json.Format(
				"{\"client_time\":%d,\"level\":%q,\"message\":%q}\n",
				int(utils.GetTime().Now().Unix()), level, msg),
			Level: logging.ALERT,
		}}
}

func (self *FlowContext) AddLogMessage(
	ctx context.Context, level string, msg string) {
	if level == logging.ALERT {
		self.sendAlertMessage(ctx, level, msg)
		return
	}

	self.mu.Lock()
	defer self.mu.Unlock()

	self.addLogMessage(level, msg)
}

func (self *FlowContext) addLogMessage(level string, msg string) {
	self.log_message_count++
	self.log_messages = append(self.log_messages, json.Format(
		"{\"client_time\":%d,\"level\":%q,\"message\":%q}\n",
		int(utils.GetTime().Now().Unix()), level, msg)...)
}

func (self *FlowContext) NextUploadId() int64 {
	new_id := int64(atomic.AddInt32(&self.upload_id, 1))
	return new_id - 1
}

// Queries are run in parallel and maintain their own stats.
func (self *FlowContext) NewResponder(
	request *actions_proto.VQLCollectorArgs) (context.Context, *FlowResponder) {

	self.mu.Lock()
	defer self.mu.Unlock()

	// Done in the Close() method.
	self.wg.Add(1)
	responder := newFlowResponder(
		self.ctx, self.config_obj, self.wg, self.output, self)
	self.responders = append(self.responders, responder)

	return self.ctx, responder
}

// Returns some stats to send to the server. The stats are sent in a
// rate limited way - not too frequently.
func (self *FlowContext) MaybeSendStats() *crypto_proto.VeloMessage {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Only send the final message once.
	if self.final_stats_sent {
		return nil
	}

	now := uint64(utils.GetTime().Now().UnixNano() / 1000)
	last_timestamp := self.last_stats_timestamp
	if self.isFlowComplete() ||
		now-last_timestamp > self.frequency_msec {
		self.last_stats_timestamp = now
		return self.getStats()
	}
	return nil
}

// send the stats immediately.
func (self *FlowContext) sendStats() {
	if !self.final_stats_sent {
		stats := self.getStats()
		if self.final_stats_sent {
			logger := logging.GetLogger(self.config_obj, &logging.ClientComponent)
			logger.Debug("Sending final message for %v: %v",
				self.flow_id, json.MustMarshalString(stats))
		}

		select {
		case <-self.ctx.Done():
		case self.output <- stats:
		}
	}
}

// Prepare a FlowStat message to send the server.
func (self *FlowContext) getStats() *crypto_proto.VeloMessage {
	result := &crypto_proto.VeloMessage{
		SessionId: self.flow_id,
		RequestId: constants.STATS_SINK,
		FlowStats: &crypto_proto.FlowStats{},
	}

	// Fill in all the responder's stats.
	for _, r := range self.responders {
		result.FlowStats.QueryStatus = append(result.FlowStats.QueryStatus,
			r.GetStatus())
	}

	if self.isFlowComplete() {
		// Let the server know this is the final message in the flow.
		result.FlowStats.FlowComplete = true
		self.final_stats_sent = true
	}

	// Write the checkpoint file
	if self.checkpoint != "" {
		serialized, err := json.Marshal(result)
		if err == nil {
			fd, err := os.OpenFile(self.checkpoint,
				os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0660)
			if err == nil {
				fd.Write(serialized)
			}
			fd.Close()
		}
	}

	return result
}
