/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

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
	"time"

	"github.com/golang/protobuf/proto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"
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
	output chan *crypto_proto.GrrMessage

	sync.Mutex
	request    *crypto_proto.GrrMessage
	logger     *logging.LogContext
	start_time int64

	// The name of the query we are currently running.
	Artifact string
}

// NewResponder returns a new Responder.
func NewResponder(
	config_obj *config_proto.Config,
	request *crypto_proto.GrrMessage,
	output chan *crypto_proto.GrrMessage) *Responder {
	result := &Responder{
		request:    request,
		output:     output,
		logger:     logging.GetLogger(config_obj, &logging.ClientComponent),
		start_time: time.Now().UnixNano(),
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

func (self *Responder) AddResponse(
	ctx context.Context, message *crypto_proto.GrrMessage) {
	self.Lock()
	output := self.output
	self.Unlock()

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
	self.AddResponse(ctx, &crypto_proto.GrrMessage{
		Status: &crypto_proto.GrrStatus{
			Backtrace:    string(debug.Stack()),
			ErrorMessage: message,
			Status:       crypto_proto.GrrStatus_GENERIC_ERROR,
			Duration:     time.Now().UnixNano() - self.start_time,
		}})
}

func (self *Responder) Return(ctx context.Context) {
	self.AddResponse(ctx, &crypto_proto.GrrMessage{
		Status: &crypto_proto.GrrStatus{
			Status:   crypto_proto.GrrStatus_OK,
			Duration: time.Now().UnixNano() - self.start_time,
		}})
}

// Send a log message to the server.
func (self *Responder) Log(ctx context.Context, format string, v ...interface{}) {
	self.AddResponse(ctx, &crypto_proto.GrrMessage{
		RequestId: constants.LOG_SINK,
		Urgent:    true,
		LogMessage: &crypto_proto.LogMessage{
			Message:   fmt.Sprintf(format, v...),
			Timestamp: uint64(time.Now().UTC().UnixNano() / 1000),
			Artifact:  self.Artifact,
		}})
}

func (self *Responder) SessionId() string {
	return self.request.SessionId
}

// If a message was received from an old client we convert it into the
// proper form.
func NormalizeGrrMessageForBackwardCompatibility(msg *crypto_proto.GrrMessage) error {
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
		msg.Status = &crypto_proto.GrrStatus{}
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
