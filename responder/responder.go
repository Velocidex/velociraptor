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

	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

// The Responder tracks a single query with the flow.
type FlowResponder struct {
	output chan *crypto_proto.VeloMessage

	// Context and cancellation point for the query that is attached
	// to this responder.
	ctx        context.Context
	cancel     func()
	wg         *sync.WaitGroup
	config_obj *config_proto.Config

	mu     sync.Mutex
	logger *logging.LogContext

	// The status contains information about the execution of the
	// query.
	status crypto_proto.VeloStatus

	// Our parent context that is shared between all queries from the
	// same collection.
	flow_context *FlowContext
}

// A Responder manages responses for a single query. A collection (or
// flow) usually contains several queries in different requests so
// there will be several responders.
func newFlowResponder(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup,
	output chan *crypto_proto.VeloMessage,
	owner *FlowContext) *FlowResponder {

	sub_ctx, cancel := context.WithCancel(ctx)
	result := &FlowResponder{
		ctx:          sub_ctx,
		cancel:       cancel,
		wg:           wg,
		config_obj:   config_obj,
		flow_context: owner,
		output:       output,
		status: crypto_proto.VeloStatus{
			Status:      crypto_proto.VeloStatus_PROGRESS,
			FirstActive: uint64(utils.GetTime().Now().UnixNano() / 1000),
		},
	}
	return result
}

func (self *FlowResponder) Close() {
	self.cancel()
	self.wg.Done()
}

func (self *FlowResponder) FlowContext() *FlowContext {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.flow_context
}

func (self *FlowResponder) NextUploadId() int64 {
	return self.flow_context.NextUploadId()
}

func (self *FlowResponder) IsComplete() bool {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.status.Status != crypto_proto.VeloStatus_PROGRESS
}

func (self *FlowResponder) GetStatus() *crypto_proto.VeloStatus {
	self.mu.Lock()
	status := proto.Clone(&self.status).(*crypto_proto.VeloStatus)
	self.mu.Unlock()

	status.LastActive = uint64(utils.GetTime().Now().UnixNano() / 1000)

	// Duration is in milli seconds
	status.Duration = int64(status.LastActive-status.FirstActive) * 1000

	return status
}

// Gets called on each response to update the query status
func (self *FlowResponder) updateStats(message *crypto_proto.VeloMessage) {
	if message.LogMessage != nil {
		self.status.LogRows += int64(message.LogMessage.NumberOfRows)
		return
	}

	if message.FileBuffer != nil {
		self.status.UploadedBytes += int64(message.FileBuffer.DataLength)

		// if this is the first FileBuffer update, we increment the
		// number of files uploaded and set the expected length.
		if message.FileBuffer.Offset == 0 {
			self.status.UploadedFiles++
			self.status.ExpectedUploadedBytes += int64(message.FileBuffer.StoredSize)
		}
	}

	if message.VQLResponse != nil {
		self.status.ResultRows = int64(
			message.VQLResponse.QueryStartRow + message.VQLResponse.TotalRows)

		addNameWithResponse(&self.status.NamesWithResponse,
			message.VQLResponse.Query.Name)
	}
}

// Called from VQL to send a response back to the server.
func (self *FlowResponder) AddResponse(message *crypto_proto.VeloMessage) {
	self.mu.Lock()
	output := self.output
	self.updateStats(message)
	self.mu.Unlock()

	// Check flow limits. Must be done without a lock on the responder.
	if message.FileBuffer != nil {
		self.flow_context.ChargeBytes(uint64(len(message.FileBuffer.Data)))
	}
	if message.VQLResponse != nil {
		self.flow_context.ChargeRows(message.VQLResponse.TotalRows)
	}

	message.SessionId = self.flow_context.SessionId()

	select {
	case <-self.ctx.Done():
		break

	case output <- message:
	}
}

func (self *FlowResponder) RaiseError(ctx context.Context, message string) {
	// Mark the query as having an error.
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.status.Status == crypto_proto.VeloStatus_PROGRESS {
		self.status.Status = crypto_proto.VeloStatus_GENERIC_ERROR
		self.status.ErrorMessage = message
		self.status.Backtrace = string(debug.Stack())
	}
}

func (self *FlowResponder) Return(ctx context.Context) {
	// Mark the query as being successful
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.status.Status == crypto_proto.VeloStatus_PROGRESS {
		self.status.Status = crypto_proto.VeloStatus_OK
	}
}

// Send a log message to the server. We do not actually send the log
// right away, but queue it locally and combine with other log
// messages for self.flushLogMessages() to send.
func (self *FlowResponder) Log(ctx context.Context, level string, msg string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.flow_context.AddLogMessage(level, msg)
	self.status.LogRows++
}

// This is expected to be small so a linear search is ok
func addNameWithResponse(array *[]string, name string) {
	if name != "" && !utils.InString(*array, name) {
		*array = append(*array, name)
	}
}
