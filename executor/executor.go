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
package executor

import (
	"context"
	"fmt"
	"log"
	"path"
	"runtime/debug"
	"sync"

	"github.com/golang/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/actions"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/responder"
)

type Executor interface {
	// These are called by the executor code.
	ReadFromServer() *crypto_proto.GrrMessage
	SendToServer(message *crypto_proto.GrrMessage)

	// These two are called by the comms module.

	// Feed a server request to the executor for execution.
	ProcessRequest(message *crypto_proto.GrrMessage)

	// Read a single response from the executor to be sent to the server.
	ReadResponse() <-chan *crypto_proto.GrrMessage
}

// A concerete implementation of a client executor.

type ClientExecutor struct {
	Inbound  chan *crypto_proto.GrrMessage
	Outbound chan *crypto_proto.GrrMessage
	plugins  map[string]actions.ClientAction

	mu                sync.Mutex
	in_flight_context map[string]chan bool
}

// Blocks until a request is received from the server. Called by the
// Executors internal processor.
func (self *ClientExecutor) ReadFromServer() *crypto_proto.GrrMessage {
	msg := <-self.Inbound
	return msg
}

func (self *ClientExecutor) SendToServer(message *crypto_proto.GrrMessage) {
	self.Outbound <- message
}

func (self *ClientExecutor) ProcessRequest(message *crypto_proto.GrrMessage) {
	self.Inbound <- message
}

func (self *ClientExecutor) ReadResponse() <-chan *crypto_proto.GrrMessage {
	return self.Outbound
}

func makeUnknownActionResponse(req *crypto_proto.GrrMessage) *crypto_proto.GrrMessage {
	reply := &crypto_proto.GrrMessage{
		SessionId:  req.SessionId,
		RequestId:  req.RequestId,
		ResponseId: 1,
		Type:       crypto_proto.GrrMessage_STATUS,
		ClientType: crypto_proto.GrrMessage_VELOCIRAPTOR,
	}

	reply.TaskId = req.TaskId
	status := &crypto_proto.GrrStatus{
		Status: crypto_proto.GrrStatus_GENERIC_ERROR,
		ErrorMessage: fmt.Sprintf(
			"Client action '%v' not known", req.Name),
	}

	status_marshalled, err := proto.Marshal(status)
	if err == nil {
		reply.Args = status_marshalled
		reply.ArgsRdfName = "GrrStatus"
	}

	return reply
}

func (self *ClientExecutor) processRequestPlugin(
	config_obj *config_proto.Config,
	ctx context.Context,
	req *crypto_proto.GrrMessage) {

	// If we panic we need to recover and report this to the
	// server.
	defer func() {
		r := recover()
		if r != nil {
			logger := logging.GetLogger(config_obj, &logging.ClientComponent)
			logger.Error(fmt.Sprintf("Panic %v: %v",
				r, string(debug.Stack())))
		}
	}()

	// Never serve unauthenticated requests.
	if req.AuthState != crypto_proto.GrrMessage_AUTHENTICATED {
		log.Printf("Unauthenticated")
		self.SendToServer(makeUnknownActionResponse(req))
		return
	}

	flow_id := path.Base(req.SessionId)

	// Handle cancellation requests especialy.
	if req.Name == "Cancel" {
		self.mu.Lock()
		defer self.mu.Unlock()

		done, pres := self.in_flight_context[flow_id]
		if pres {
			responder := responder.NewResponder(req, self.Outbound)
			responder.Log("Received cancellation request for flow id %v",
				flow_id)

			close(done)
			delete(self.in_flight_context, flow_id)
		}
		return
	}

	plugin, pres := self.plugins[req.Name]
	if !pres {
		self.SendToServer(makeUnknownActionResponse(req))
		return
	}

	// Install a cancellation channel to allow all queries from
	// this flow to be cancelled by the server.
	self.mu.Lock()
	defer self.mu.Unlock()

	done, pres := self.in_flight_context[flow_id]
	if !pres {
		done = make(chan bool)
		self.in_flight_context[flow_id] = done
	}

	sub_ctx, cancel := context.WithCancel(ctx)
	go func() {
		<-done
		cancel()
	}()

	// Run the plugin in the other thread and drain its messages
	// to send to the server.
	go func() {
		plugin.Run(config_obj, sub_ctx, req, self.Outbound)

		// Remove cancellation channel.
		self.mu.Lock()
		defer self.mu.Unlock()

		done, pres := self.in_flight_context[flow_id]
		if pres {
			close(done)
			delete(self.in_flight_context, flow_id)
		}
	}()
}

func NewClientExecutor(config_obj *config_proto.Config) (*ClientExecutor, error) {
	result := &ClientExecutor{
		Inbound:  make(chan *crypto_proto.GrrMessage),
		Outbound: make(chan *crypto_proto.GrrMessage),
		plugins:  actions.GetClientActionsMap(),

		in_flight_context: make(map[string]chan bool),
	}

	go func() {
		logger := logging.GetLogger(config_obj, &logging.ClientComponent)

		for {
			// Pump messages from input channel and
			// process each request.
			req := result.ReadFromServer()

			// Ignore unauthenticated messages - the
			// server should never send us those.
			if req.AuthState == crypto_proto.GrrMessage_AUTHENTICATED {
				// Each request has its own context.
				ctx := context.Background()
				logger.Info("Received request: %v", req)

				// Process the request asynchronously.
				go result.processRequestPlugin(config_obj, ctx, req)
			}
		}
	}()

	return result, nil
}
