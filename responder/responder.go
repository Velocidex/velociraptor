/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

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
	"regexp"
	"runtime/debug"
	"sync"

	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	defaultLogErrorRegex = regexp.MustCompile(constants.VQL_ERROR_REGEX)
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

	mu sync.Mutex

	// The status contains information about the execution of the
	// query.
	status *crypto_proto.VeloStatus

	// Our parent context that is shared between all queries from the
	// same collection.
	flow_context *FlowContext

	completed bool

	logErrorRegex *regexp.Regexp
}

// A Responder manages responses for a single query. A collection (or
// flow) usually contains several queries in different requests so
// there will be several responders.
func newFlowResponder(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup,
	output chan *crypto_proto.VeloMessage,
	req *crypto_proto.FlowRequest,
	owner *FlowContext) *FlowResponder {

	sub_ctx, cancel := context.WithCancel(ctx)
	result := &FlowResponder{
		ctx:          sub_ctx,
		cancel:       cancel,
		wg:           wg,
		config_obj:   config_obj,
		flow_context: owner,
		output:       output,
		status: &crypto_proto.VeloStatus{
			Status:      crypto_proto.VeloStatus_PROGRESS,
			FirstActive: uint64(utils.GetTime().Now().UnixNano() / 1000),
		},
		logErrorRegex: defaultLogErrorRegex,
	}

	if req.LogErrorRegex != "" {
		re, err := regexp.Compile(req.LogErrorRegex)
		if err == nil {
			result.logErrorRegex = re
		}
	}

	return result
}

func (self *FlowResponder) SetStatus(s *crypto_proto.VeloStatus) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.status = proto.Clone(s).(*crypto_proto.VeloStatus)
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

	return self.completed
}

func (self *FlowResponder) GetStatus() *crypto_proto.VeloStatus {
	self.mu.Lock()

	if !self.completed {
		self.status.LastActive = uint64(utils.GetTime().Now().UnixNano() / 1000)

		// Duration is in milli seconds
		self.status.Duration = int64(self.status.LastActive-self.status.FirstActive) * 1000
	}

	status := proto.Clone(self.status).(*crypto_proto.VeloStatus)
	self.mu.Unlock()

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
			self.status.ExpectedUploadedBytes += int64(message.FileBuffer.Size)
		}
	}

	if message.VQLResponse != nil {
		self.status.ResultRows += int64(message.VQLResponse.TotalRows)

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
		uncompressed_size := uint64(len(message.FileBuffer.Data))

		if uncompressed_size > 0 &&
			self.flow_context.req.Compression == crypto_proto.FlowRequest_ZLIB {
			compressed, err := utils.Compress(message.FileBuffer.Data)
			if err == nil {
				message.FileBuffer.UncompressedLength = uncompressed_size
				message.FileBuffer.Data = compressed
			}
		}

		err := self.flow_context.ChargeBytes(
			uint64(message.FileBuffer.DataLength))
		if err != nil {
			// If we exceeded the limits cancel the entire
			// collection.
			self.RaiseError(self.ctx, err.Error())
			self.flow_context.Cancel()
		}
	}

	if message.VQLResponse != nil {
		name := ""
		if message.VQLResponse.Query != nil {
			name = message.VQLResponse.Query.Name
		}

		uncompressed_size := uint64(len(message.VQLResponse.JSONLResponse))
		if uncompressed_size > 0 &&
			self.flow_context.req.Compression == crypto_proto.FlowRequest_ZLIB {

			compressed, err := utils.Compress(
				[]byte(message.VQLResponse.JSONLResponse))
			if err == nil {
				message.VQLResponse.UncompressedSize = uncompressed_size
				message.VQLResponse.CompressedJsonResponse = compressed
				message.VQLResponse.JSONLResponse = ""
				message.VQLResponse.ByteOffset = self.flow_context.GetJSONLBytes(name)
			}
		}
		self.flow_context.ChargeJSONLBytes(name, uncompressed_size)

		err := self.flow_context.ChargeRows(message.VQLResponse.TotalRows)
		if err != nil {
			self.RaiseError(self.ctx, err.Error())
			self.flow_context.Cancel()
		}
	}

	message.SessionId = self.flow_context.SessionId()

	select {
	case <-self.ctx.Done():
		break

	case output <- message:
	}
}

/*
Mark an error in this collection.

In previous versions marking an error would flag the result of the
collection as error immediately but the collection continues to
run.

Our concept of what an error represents has evolved over time. It
is difficult to know what to do when a VQL query enounters an
error or even what an error means.

For example, if the VQL query tries to parse a certain file but
fails to parse the file - is this an error? it might be depending
on context. Most of the time we want to just report the issue and
move on.

VQL always continues running when encountering an error. This
simplifies writing the queries (because we dont need to deal with
errors all the time). But we need to report the error, usually via
the query log.

When a user collects an artifact, the GUI shows the success status
of the artifact. What consitutes success is really subjecting and
depends on the context of the artifact.

Because we dont really know we leave it to the VQL to determine if
the collection should be marked as failed. If the VQL logs any
message at ERROR level, we deem the collection to have
failed. However, the query is **NOT** aborted - it keeps running
and may still produce useful results.

In this way the error status of a collection is more like a flag -
it simply represents that the user should inspect the collection
more closely to see if the data returned is still useful.

For example, if the VQL query references an unknown symbol (Symbol
not found error), this usually represents that the query has
invalid syntax or some error in it (e.g. a field is mistyped). We
generally report this error at the ERROR log level which causes the
collection to fail.

However the collection itself continues running as normal (unknown
symbols are represented by NULL). The collection may still contain
useful data, even if it is marked as failed. The collection will be
allowed to run to completion.

This means that an ERROR is simply an advisory flag to mark that the
collection should be looked at more closely (by inspecting the query
log).

The RaiseError() function will be called when the VQL encounters an
error (it may be called multiple times).

We record the first error reported but leave the collection in the
PROGRESS state.
*/
func (self *FlowResponder) RaiseError(ctx context.Context, message string) {
	// Mark the query as having an error.
	self.mu.Lock()
	defer self.mu.Unlock()

	// Mark only the first error in the status error message.
	if self.status.ErrorMessage == "" {
		if message == "" {
			message = "Generic Error"
		}
		self.status.ErrorMessage = message
		self.status.Backtrace = string(debug.Stack())
	}
}

func (self *FlowResponder) Cancel(ctx context.Context) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.status.ErrorMessage == "" {
		self.status.ErrorMessage = "Cancelled"
		self.status.Backtrace = ""
	}
	self.status.Status = crypto_proto.VeloStatus_GENERIC_ERROR
	self.completed = true
}

/*
The Return() function represents the end of the query.

We finalize the status to either an OK or ERROR status depending on
the error message.
*/
func (self *FlowResponder) Return(ctx context.Context) {
	// Mark the query as being successful
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.status.Status == crypto_proto.VeloStatus_PROGRESS {
		if self.status.ErrorMessage == "" {
			self.status.Status = crypto_proto.VeloStatus_OK
		} else {
			self.status.Status = crypto_proto.VeloStatus_GENERIC_ERROR
		}
	}

	// Only when the query is completed, we call Return()
	self.completed = true
	self.status.LastActive = uint64(utils.GetTime().Now().UnixNano() / 1000)

	// Duration is in milli seconds
	self.status.Duration = int64(self.status.LastActive-self.status.FirstActive) * 1000
}

// Send a log message to the server. We do not actually send the log
// right away, but queue it locally and combine with other log
// messages for self.flushLogMessages() to send.
func (self *FlowResponder) Log(ctx context.Context, level string, msg string) {
	// If the log message looks like an error then mark it as an
	// error.
	if level != logging.ERROR &&
		self.logErrorRegex.FindStringIndex(msg) != nil {
		level = logging.ERROR
	}

	// We dont need to hold the lock because we are just delegating to
	// the flow context.
	self.flow_context.AddLogMessage(ctx, level, msg)

	// Capture the first message at error level.
	// FIXME: Support server provided error regex patterns
	if level == logging.ERROR {
		self.RaiseError(ctx, msg)
	}

	self.mu.Lock()
	self.status.LogRows++
	self.mu.Unlock()
}

// This is expected to be small so a linear search is ok
func addNameWithResponse(array *[]string, name string) {
	if name != "" && !utils.InString(*array, name) {
		*array = append(*array, name)
	}
}
