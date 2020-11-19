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

type Responder struct {
	output chan *crypto_proto.GrrMessage

	sync.Mutex
	request    *crypto_proto.GrrMessage
	next_id    uint64
	logger     *logging.LogContext
	start_time int64
}

// NewResponder returns a new Responder.
func NewResponder(
	config_obj *config_proto.Config,
	request *crypto_proto.GrrMessage,
	output chan *crypto_proto.GrrMessage) *Responder {
	result := &Responder{
		request:    request,
		next_id:    0,
		output:     output,
		logger:     logging.GetLogger(config_obj, &logging.ClientComponent),
		start_time: time.Now().UnixNano(),
	}
	return result
}

func (self *Responder) AddResponse(message *crypto_proto.GrrMessage) {
	self.Lock()
	defer self.Unlock()

	message.SessionId = self.request.SessionId
	message.Urgent = self.request.Urgent
	message.ResponseId = self.next_id
	self.next_id++
	if message.RequestId == 0 {
		message.RequestId = self.request.RequestId
	}
	message.TaskId = self.request.TaskId

	if self.output != nil {
		select {
		case self.output <- message:
		}
	}
}

func (self *Responder) RaiseError(message string) {
	self.AddResponse(&crypto_proto.GrrMessage{
		Status: &crypto_proto.GrrStatus{
			Backtrace:    string(debug.Stack()),
			ErrorMessage: message,
			Status:       crypto_proto.GrrStatus_GENERIC_ERROR,
			Duration:     time.Now().UnixNano() - self.start_time,
		}})
}

func (self *Responder) Return() {
	self.AddResponse(&crypto_proto.GrrMessage{
		Status: &crypto_proto.GrrStatus{
			Status:   crypto_proto.GrrStatus_OK,
			Duration: time.Now().UnixNano() - self.start_time,
		}})
}

// Send a log message to the server.
func (self *Responder) Log(format string, v ...interface{}) {
	self.AddResponse(&crypto_proto.GrrMessage{
		RequestId: constants.LOG_SINK,
		Urgent:    true,
		LogMessage: &crypto_proto.LogMessage{
			Message:   fmt.Sprintf(format, v...),
			Timestamp: uint64(time.Now().UTC().UnixNano() / 1000),
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
