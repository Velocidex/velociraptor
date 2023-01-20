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
package executor

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"sync"
	"time"

	"www.velocidex.com/golang/velociraptor/actions"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/utils"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/responder"
)

type Executor interface {
	ClientId() string

	// These are called by the executor code.
	ReadFromServer() *crypto_proto.VeloMessage
	SendToServer(message *crypto_proto.VeloMessage)

	// These two are called by the comms module.

	// Feed a server request to the executor for execution.
	ProcessRequest(
		ctx context.Context,
		message *crypto_proto.VeloMessage)

	// Read a single response from the executor to be sent to the server.
	ReadResponse() <-chan *crypto_proto.VeloMessage
}

// A concerete implementation of a client executor.

type ClientExecutor struct {
	client_id string

	Inbound  chan *crypto_proto.VeloMessage
	Outbound chan *crypto_proto.VeloMessage

	config_obj *config_proto.Config

	concurrency *utils.Concurrency
}

func (self *ClientExecutor) ClientId() string {
	return self.client_id
}

// Blocks until a request is received from the server. Called by the
// Executors internal processor.
func (self *ClientExecutor) ReadFromServer() *crypto_proto.VeloMessage {
	msg := <-self.Inbound
	return msg
}

func (self *ClientExecutor) SendToServer(message *crypto_proto.VeloMessage) {
	self.Outbound <- message
}

func (self *ClientExecutor) ProcessRequest(
	ctx context.Context,
	message *crypto_proto.VeloMessage) {
	self.Inbound <- message
}

func (self *ClientExecutor) ReadResponse() <-chan *crypto_proto.VeloMessage {
	return self.Outbound
}

func makeErrorResponse(output chan *crypto_proto.VeloMessage,
	req *crypto_proto.VeloMessage, message string) {
	output <- &crypto_proto.VeloMessage{
		SessionId: req.SessionId,
		RequestId: constants.LOG_SINK,
		LogMessage: &crypto_proto.LogMessage{
			Message:   message,
			Level:     logging.ERROR,
			Timestamp: uint64(time.Now().UTC().UnixNano() / 1000),
		},
	}

	output <- &crypto_proto.VeloMessage{
		SessionId:  req.SessionId,
		RequestId:  req.RequestId,
		ResponseId: 1,
		Status: &crypto_proto.VeloStatus{
			Status:       crypto_proto.VeloStatus_GENERIC_ERROR,
			ErrorMessage: message,
		},
	}
}

func (self *ClientExecutor) processRequestPlugin(
	config_obj *config_proto.Config,
	ctx context.Context,
	req *crypto_proto.VeloMessage) {

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
	if req.AuthState != crypto_proto.VeloMessage_AUTHENTICATED {
		log.Printf("Unauthenticated")
		makeErrorResponse(self.Outbound,
			req, fmt.Sprintf("Unauthenticated message received: %v.", req))
		return
	}

	flow_manager := responder.GetFlowManager(ctx, config_obj)

	if req.Cancel != nil {
		// Only log when the flow is not already cancelled.
		if !flow_manager.IsCancelled(req.SessionId) {
			flow_manager.Cancel(ctx, req.SessionId)

			makeErrorResponse(self.Outbound,
				req, fmt.Sprintf("Cancelled all inflight queries for flow %v",
					req.SessionId))
		}
		return
	}

	// This is the old deprecated VQLClientAction that is sent for old
	// client compatibility. New clients ignore this and only process
	// a FlowRequest message.
	if req.VQLClientAction != nil {
		return
	}

	if req.FlowRequest != nil {
		self.ProcessFlowRequest(ctx, config_obj, req)
		return
	}

	if req.UpdateEventTable != nil {
		// Handle the requests. This used to be a plugin registration
		// process but there are very few plugins any more and so it
		// is easier to hard code this.
		responder_obj := responder.NewResponder(ctx, config_obj, req, self.Outbound)
		defer responder_obj.Close()

		// Each request has its own context.
		flow_context := flow_manager.FlowContext(req)
		query_ctx, closer := flow_context.NewQueryContext(responder_obj)
		defer closer()

		actions.UpdateEventTable{}.Run(
			config_obj, query_ctx, responder_obj, req.UpdateEventTable)
		return
	}

	// This action is deprecated now.
	if req.UpdateForeman != nil {
		return
	}

	makeErrorResponse(self.Outbound,
		req, fmt.Sprintf("Unsupported payload for message: %v", req))
}

func NewClientExecutor(
	ctx context.Context,
	client_id string,
	config_obj *config_proto.Config) (*ClientExecutor, error) {

	level := int(config_obj.Client.Concurrency)
	if level == 0 {
		level = 2
	}

	result := &ClientExecutor{
		client_id:   client_id,
		Inbound:     make(chan *crypto_proto.VeloMessage, 10),
		Outbound:    make(chan *crypto_proto.VeloMessage, 10),
		concurrency: utils.NewConcurrencyControl(level, time.Hour),
		config_obj:  config_obj,
	}

	// Drain messages from server and execute them, pushing
	// results to the output channel.
	go func() {
		// Keep this open to avoid sending on close
		// channels. The executed queries should finish by
		// themselves when the context is done.

		// defer close(result.Outbound)

		// Do not exit until all goroutines have finished.
		wg := &sync.WaitGroup{}
		defer wg.Wait()

		logger := logging.GetLogger(config_obj, &logging.ClientComponent)

		for {
			select {
			// Context is cancelled wrap this up and go
			// home.  (Never normally called in the client
			// but used in tests to cleanup.)
			case <-ctx.Done():
				return

			// Pump messages from input channel and
			// process each request.
			case req, ok := <-result.Inbound:
				if !ok {
					return
				}

				// Ignore unauthenticated messages - the
				// server should never send us those.
				if req.AuthState == crypto_proto.VeloMessage_AUTHENTICATED {
					wg.Add(1)
					go func() {
						defer wg.Done()

						logger.Debug("Received request: %v", req)
						result.processRequestPlugin(config_obj, ctx, req)
					}()
				}
			}
		}
	}()

	return result, nil
}
