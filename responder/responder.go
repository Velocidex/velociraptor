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
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	inc_mu     sync.Mutex
	last_value uint64
)

// Response ID used to be incremental but now artifacts are collected
// in parallel each collection needs to advance the response id
// forward. We therefore remember the last id and ensure response id
// is monotonically incremental but time based.
func getIncValue() uint64 {
	inc_mu.Lock()
	defer inc_mu.Unlock()

	value := uint64(time.Now().UnixNano())
	if value <= last_value {
		value = last_value + 1
	}
	last_value = value
	return last_value
}

type Responder struct {
	output chan *crypto_proto.VeloMessage

	sync.Mutex
	request *crypto_proto.VeloMessage
	logger  *logging.LogContext

	start_time int64
	log_id     int32

	// The name of the query we are currently running.
	Artifact string

	// Keep track of stats to fill into the Status message
	names_with_response []string
	log_rows            int64
	result_rows         int64
}

// NewResponder returns a new Responder.
func NewResponder(
	config_obj *config_proto.Config,
	request *crypto_proto.VeloMessage,
	output chan *crypto_proto.VeloMessage) *Responder {
	result := &Responder{
		request:    request,
		output:     output,
		logger:     logging.GetLogger(config_obj, &logging.ClientComponent),
		start_time: time.Now().UnixNano(),
	}

	if request.VQLClientAction != nil {
		for _, q := range request.VQLClientAction.Query {
			if q.Name != "" {
				result.Artifact = q.Name
			}
		}
	}

	return result
}

func (self *Responder) Copy() *Responder {
	return &Responder{
		request:    self.request,
		output:     self.output,
		logger:     self.logger,
		start_time: time.Now().UnixNano(),
	}
}

func (self *Responder) updateStats(message *crypto_proto.VeloMessage) {
	if message.LogMessage != nil {
		self.log_rows++
		return
	}

	if message.VQLResponse != nil {
		self.result_rows = int64(message.VQLResponse.QueryStartRow +
			message.VQLResponse.TotalRows)

		if message.VQLResponse.Query != nil && !utils.InString(
			self.names_with_response, message.VQLResponse.Query.Name) {
			self.names_with_response = append(self.names_with_response,
				message.VQLResponse.Query.Name)
		}
	}
}

func (self *Responder) getStatus() *crypto_proto.VeloStatus {
	self.Lock()
	defer self.Unlock()

	status := &crypto_proto.VeloStatus{
		NamesWithResponse: self.names_with_response,
		LogRows:           self.log_rows,
		ResultRows:        self.result_rows,
		Duration:          time.Now().UnixNano() - self.start_time,
		Artifact:          self.Artifact,
	}

	if self.request.VQLClientAction != nil {
		status.QueryId = self.request.VQLClientAction.QueryId
		status.TotalQueries = self.request.VQLClientAction.TotalQueries
	}

	return status
}

func (self *Responder) AddResponse(
	ctx context.Context, message *crypto_proto.VeloMessage) {
	self.Lock()
	output := self.output
	self.updateStats(message)
	self.Unlock()

	message.QueryId = self.request.QueryId
	message.SessionId = self.request.SessionId
	message.Urgent = self.request.Urgent
	message.ResponseId = getIncValue()
	if message.RequestId == 0 {
		message.RequestId = self.request.RequestId
	}
	message.TaskId = self.request.TaskId

	if output != nil {
		select {
		case <-ctx.Done():
			break

		case output <- message:
		}
	}
}

func (self *Responder) RaiseError(ctx context.Context, message string) {
	status := self.getStatus()
	status.Backtrace = string(debug.Stack())
	status.ErrorMessage = message
	status.Status = crypto_proto.VeloStatus_GENERIC_ERROR
	status.NamesWithResponse = self.names_with_response
	status.Artifact = self.Artifact

	self.AddResponse(ctx, &crypto_proto.VeloMessage{Status: status})
}

func (self *Responder) Return(ctx context.Context) {
	status := self.getStatus()
	status.Status = crypto_proto.VeloStatus_OK

	self.AddResponse(ctx, &crypto_proto.VeloMessage{Status: status})
}

// Send a log message to the server.
func (self *Responder) Log(ctx context.Context, level string,
	format string, v ...interface{}) {
	self.AddResponse(ctx, &crypto_proto.VeloMessage{
		RequestId: constants.LOG_SINK,
		LogMessage: &crypto_proto.LogMessage{
			Id:        int64(atomic.LoadInt32(&self.log_id)),
			Message:   fmt.Sprintf(format, v...),
			Timestamp: uint64(time.Now().UTC().UnixNano() / 1000),
			Artifact:  self.Artifact,
			Level:     level,
		}})
	atomic.AddInt32(&self.log_id, 1)
}

func (self *Responder) SessionId() string {
	return self.request.SessionId
}

// If a message was received from an old client we convert it into the
// proper form.
func NormalizeVeloMessageForBackwardCompatibility(msg *crypto_proto.VeloMessage) error {
	if msg.UpdateEventTable != nil ||
		msg.VQLClientAction != nil ||
		msg.Cancel != nil ||
		msg.UpdateForeman != nil ||
		msg.Status != nil ||
		msg.ForemanCheckin != nil ||
		msg.FileBuffer != nil ||
		msg.CSR != nil ||
		msg.VQLResponse != nil ||
		msg.LogMessage != nil {
		return nil
	}

	switch msg.ArgsRdfName {
	case "":
		return nil

	// Messages from client to server here.
	case "GrrStatus":
		msg.Status = &crypto_proto.VeloStatus{}
		return proto.Unmarshal(msg.Args, msg.Status)

	case "ForemanCheckin":
		msg.ForemanCheckin = &actions_proto.ForemanCheckin{}
		return proto.Unmarshal(msg.Args, msg.ForemanCheckin)

	case "FileBuffer":
		msg.FileBuffer = &actions_proto.FileBuffer{}
		return proto.Unmarshal(msg.Args, msg.FileBuffer)

	case "Certificate":
		msg.CSR = &crypto_proto.Certificate{}
		return proto.Unmarshal(msg.Args, msg.CSR)

	case "VQLResponse":
		msg.VQLResponse = &actions_proto.VQLResponse{}
		return proto.Unmarshal(msg.Args, msg.VQLResponse)

	case "LogMessage":
		msg.LogMessage = &crypto_proto.LogMessage{}
		return proto.Unmarshal(msg.Args, msg.LogMessage)

	default:
		panic("Unable to handle message " + msg.String())
	}
}
