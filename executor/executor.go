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
	"runtime/debug"

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

func makeErrorResponse(req *crypto_proto.GrrMessage, message string) *crypto_proto.GrrMessage {
	return &crypto_proto.GrrMessage{
		SessionId:  req.SessionId,
		RequestId:  req.RequestId,
		ResponseId: 1,
		Status: &crypto_proto.GrrStatus{
			Status:       crypto_proto.GrrStatus_GENERIC_ERROR,
			ErrorMessage: message,
		},
	}
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
		self.Outbound <- makeErrorResponse(
			req, fmt.Sprintf("Unauthenticated message received: %v.", req))
		return
	}

	// Handle the requests. This used to be a plugin registration
	// process but there are very few plugins any more and so it
	// is easier to hard code this.
	responder := responder.NewResponder(config_obj, req, self.Outbound)

	if req.VQLClientAction != nil {
		go actions.VQLClientAction{}.StartQuery(
			config_obj, ctx, responder, req.VQLClientAction)
		return
	}

	if req.UpdateEventTable != nil {
		go actions.UpdateEventTable{}.Run(
			config_obj, ctx, responder, req.UpdateEventTable)
		return
	}

	if req.UpdateForeman != nil {
		go actions.UpdateForeman{}.Run(
			config_obj, ctx, responder, req.UpdateForeman)
		return
	}

	self.Outbound <- makeErrorResponse(
		req, fmt.Sprintf("Unsupported payload for message: %v", req))
}

func NewClientExecutor(config_obj *config_proto.Config) (*ClientExecutor, error) {
	result := &ClientExecutor{
		Inbound:  make(chan *crypto_proto.GrrMessage),
		Outbound: make(chan *crypto_proto.GrrMessage),
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
