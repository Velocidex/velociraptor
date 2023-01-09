/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2022 Rapid7 Inc.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package responder

import (
	"context"
	"runtime/debug"
	"sync"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

type Responder struct {
	output chan *crypto_proto.VeloMessage

	// Context and cancellation point for the query that is attached
	// to this responder.
	ctx        context.Context
	cancel     func()
	config_obj *config_proto.Config

	sync.Mutex
	request *crypto_proto.VeloMessage
	logger  *logging.LogContext

	// When the query started.
	start_time int64

	// The name of the query we are currently running.
	Artifact string

	// Keep track of stats to fill into the Status message. NOTE:
	// These stats track the specific query, while the flow stats
	// track the complete flow which may contain multiple queries.
	names_with_response []string
	log_rows            uint64
	uploaded_rows       uint64
	result_rows         uint64

	// A context that is shared between all queries from the same
	// collection.
	flow_context *FlowContext
}

// A Responder manages responses for a single query. A collection (or
// flow) usually contains several queries in different requests so
// there will be several responders.
func NewResponder(
	ctx context.Context,
	config_obj *config_proto.Config,
	request *crypto_proto.VeloMessage,
	output chan *crypto_proto.VeloMessage) *Responder {

	sub_ctx, cancel := context.WithCancel(ctx)

	result := &Responder{
		ctx:        sub_ctx,
		cancel:     cancel,
		config_obj: config_obj,
		request:    request,
		output:     output,
		logger:     logging.GetLogger(config_obj, &logging.ClientComponent),
		start_time: utils.GetTime().Now().UnixNano(),
	}

	if request.VQLClientAction != nil {
		for _, q := range request.VQLClientAction.Query {
			if q.Name != "" {
				result.Artifact = q.Name
			}
		}
	}

	go func() {
		batch_delay := uint64(1)
		if config_obj.Client != nil &&
			config_obj.Client.DefaultLogBatchTime > 0 {
			batch_delay = config_obj.Client.DefaultLogBatchTime
		}

		for {
			select {
			case <-sub_ctx.Done():
				return
			case <-time.After(time.Second * time.Duration(batch_delay)):
				result.flushLogMessages(ctx)
			}
		}
	}()

	return result
}

func (self *Responder) Close() {
	self.flushLogMessages(self.ctx)
	self.cancel()
}

func (self *Responder) Copy() *Responder {
	return &Responder{
		ctx:        self.ctx,
		config_obj: self.config_obj,
		request:    self.request,
		output:     self.output,
		logger:     self.logger,
		start_time: utils.GetTime().Now().UnixNano(),
	}
}

// Ensure a valid flow context exists
func (self *Responder) GetFlowContext() *FlowContext {
	flow_manager := GetFlowManager(self.ctx, self.config_obj)
	return flow_manager.FlowContext(self.request)
}

func (self *Responder) getStatus() *crypto_proto.VeloStatus {
	self.Lock()
	defer self.Unlock()

	status := &crypto_proto.VeloStatus{
		NamesWithResponse: self.names_with_response,
		LogRows:           int64(self.log_rows),
		UploadedFiles:     int64(self.uploaded_rows),
		ResultRows:        int64(self.result_rows),
		Duration:          utils.GetTime().Now().UnixNano() - self.start_time,
		Artifact:          self.Artifact,
	}

	if self.request.VQLClientAction != nil {
		status.QueryId = self.request.VQLClientAction.QueryId
		status.TotalQueries = self.request.VQLClientAction.TotalQueries
	}

	return status
}

func (self *Responder) updateStats(message *crypto_proto.VeloMessage) {
	if message.LogMessage != nil {
		self.log_rows += message.LogMessage.NumberOfRows
		return
	}

	if message.FileBuffer != nil {
		self.uploaded_rows++
	}

	if message.VQLResponse != nil {
		self.result_rows = message.VQLResponse.QueryStartRow +
			message.VQLResponse.TotalRows

		if message.VQLResponse.Query != nil && !utils.InString(
			self.names_with_response, message.VQLResponse.Query.Name) {
			self.names_with_response = append(self.names_with_response,
				message.VQLResponse.Query.Name)
		}
	}
}

func (self *Responder) AddResponse(message *crypto_proto.VeloMessage) {
	self.Lock()
	output := self.output
	self.updateStats(message)
	self.Unlock()

	// Update the flow stats
	self.GetFlowContext().Stats.UpdateStats(message)

	message.QueryId = self.request.QueryId
	message.SessionId = self.request.SessionId
	message.Urgent = self.request.Urgent
	message.TaskId = self.request.TaskId

	select {
	case <-self.ctx.Done():
		break

	case output <- message:
	}
}

func (self *Responder) RaiseError(ctx context.Context, message string) {
	status := self.getStatus()
	status.Backtrace = string(debug.Stack())
	status.ErrorMessage = message
	status.Status = crypto_proto.VeloStatus_GENERIC_ERROR
	status.NamesWithResponse = self.names_with_response
	status.Artifact = self.Artifact

	self.GetFlowContext().Stats.UpdateStats(
		&crypto_proto.VeloMessage{Status: status})
}

func (self *Responder) Return(ctx context.Context) {
	status := self.getStatus()
	status.Status = crypto_proto.VeloStatus_OK

	self.GetFlowContext().Stats.UpdateStats(
		&crypto_proto.VeloMessage{Status: status})
}

// Send a log message to the server.
func (self *Responder) Log(ctx context.Context, level string, msg string) {
	self.GetFlowContext().AddLogMessage(level, msg, self.Artifact)
}

func (self *Responder) flushLogMessages(ctx context.Context) {
	buf, id, count, error_message := self.GetFlowContext().GetLogMessages()
	if len(buf) > 0 {
		self.AddResponse(&crypto_proto.VeloMessage{
			RequestId: constants.LOG_SINK,
			LogMessage: &crypto_proto.LogMessage{
				Id:           int64(id),
				NumberOfRows: count,
				Jsonl:        string(buf),
				ErrorMessage: error_message,
				Artifact:     self.Artifact,
			}})
	}

	// Maybe send a periodic stats update
	stats := self.GetFlowContext().Stats.MaybeSendStats()
	if stats != nil && !stats.FlowComplete {
		self.AddResponse(&crypto_proto.VeloMessage{
			RequestId: constants.STATS_SINK,
			FlowStats: stats,
		})
	}
}

func (self *Responder) SessionId() string {
	return self.request.SessionId
}
