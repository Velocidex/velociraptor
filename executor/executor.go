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
package executor

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/actions"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/responder"
)

type Executor interface {
	ClientId() string

	// These are called by the executor code.
	SendToServer(message *crypto_proto.VeloMessage)

	// These two are called by the comms module.

	// Feed a server request to the executor for execution.
	ProcessRequest(
		ctx context.Context,
		message *crypto_proto.VeloMessage)

	// Read a single response from the executor to be sent to the server.
	ReadResponse() <-chan *crypto_proto.VeloMessage

	FlowManager() *responder.FlowManager
	EventManager() *actions.EventTable

	GetClientInfo() *actions_proto.ClientInfo

	Nanny() *NannyService
}

// A concerete implementation of a client executor.

type ClientExecutor struct {
	client_id string

	ctx      context.Context
	wg       *sync.WaitGroup
	Inbound  chan *crypto_proto.VeloMessage
	Outbound chan *crypto_proto.VeloMessage

	config_obj *config_proto.Config

	concurrency *utils.Concurrency

	flow_manager  *responder.FlowManager
	event_manager *actions.EventTable
}

func (self *ClientExecutor) Nanny() *NannyService {
	return Nanny
}

func (self *ClientExecutor) GetClientInfo() *actions_proto.ClientInfo {
	return actions.GetClientInfo(self.ctx, self.config_obj)
}

func (self *ClientExecutor) FlowManager() *responder.FlowManager {
	return self.flow_manager
}

func (self *ClientExecutor) EventManager() *actions.EventTable {
	return self.event_manager
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
			logger.Error("Panic %v: %v", r, string(debug.Stack()))
		}
	}()

	// Never serve unauthenticated requests.
	if req.AuthState != crypto_proto.VeloMessage_AUTHENTICATED {
		responder.MakeErrorResponse(self.Outbound,
			req.SessionId,
			fmt.Sprintf("Unauthenticated message received: %v.", req))
		return
	}

	if req.Cancel != nil {
		logger := logging.GetLogger(config_obj, &logging.ClientComponent)
		logger.Info("Received cancel for flow %v", req.SessionId)

		// Try to cancel the flow and send a message if it worked
		self.flow_manager.Cancel(ctx, req.SessionId)
		return
	}

	if req.FlowRequest != nil {
		self.ProcessFlowRequest(ctx, config_obj, req)
		return
	}

	if req.ResumeTransactions != nil {
		self.ResumeTransactions(ctx, config_obj, req)
		return
	}

	if req.UpdateEventTable != nil {
		self.event_manager.UpdateEventTable(
			self.ctx, self.wg, config_obj,
			self.Outbound, req.UpdateEventTable)
		return
	}

	// This is the old deprecated VQLClientAction that is sent for old
	// client compatibility. New clients ignore this and only process
	// a FlowRequest message.
	if req.VQLClientAction != nil {
		return
	}

	// This action is deprecated now.
	if req.UpdateForeman != nil {
		return
	}

	if req.FlowStatsRequest != nil {
		self.ProcessStatRequest(ctx, config_obj, req)
		return
	}

	responder.MakeErrorResponse(self.Outbound,
		req.SessionId, fmt.Sprintf(
			"Unsupported payload for message: %v", json.MustMarshalString(req)))
}

func NewClientExecutor(
	ctx context.Context,
	client_id string,
	config_obj *config_proto.Config) (*ClientExecutor, error) {

	level := int(config_obj.Client.Concurrency)
	if level == 0 {
		level = 2
	}

	wg := &sync.WaitGroup{}
	self := &ClientExecutor{
		ctx:          ctx,
		client_id:    client_id,
		Inbound:      make(chan *crypto_proto.VeloMessage, 10),
		Outbound:     make(chan *crypto_proto.VeloMessage, 10),
		concurrency:  utils.NewConcurrencyControl(level, time.Hour),
		wg:           wg,
		config_obj:   config_obj,
		flow_manager: responder.NewFlowManager(ctx, config_obj, client_id),
	}

	// Install and initialize the event manager
	self.event_manager = actions.NewEventTable(ctx, wg, config_obj)
	self.event_manager.StartFromWriteback(ctx, wg, config_obj, self.Outbound)

	// Drain messages from server and execute them, pushing
	// results to the output channel.
	go func() {
		// Keep this open to avoid sending on close
		// channels. The executed queries should finish by
		// themselves when the context is done.

		// Do not exit until all goroutines have finished.
		defer self.wg.Wait()

		logger := logging.GetLogger(config_obj, &logging.ClientComponent)

		for {
			select {
			// Context is cancelled wrap this up and go
			// home.  (Never normally called in the client
			// but used in tests to cleanup.)
			case <-ctx.Done():
				return

			// Pump messages from input channel and process each
			// request.
			case req, ok := <-self.Inbound:
				if !ok {
					return
				}

				// The server sets both VQLClientAction and
				// FlowRequest members on some messages for backwards
				// compatibility. We strip the old VQLClientAction
				// because we dont use it.

				// This message has VQLClientAction and no FlowRequest
				// - we can not use it - it is for the old clients.
				if req.VQLClientAction != nil && req.FlowRequest == nil {
					continue
				}

				// Ignore unauthenticated messages - the
				// server should never send us those.
				if req.AuthState == crypto_proto.VeloMessage_AUTHENTICATED {
					self.wg.Add(1)
					go func() {
						defer self.wg.Done()

						DebugMessage(req, logger)
						self.processRequestPlugin(config_obj, ctx, req)
					}()
				}
			}
		}
	}()

	return self, nil
}

func DebugMessage(req *crypto_proto.VeloMessage, logger *logging.LogContext) {
	if logger.IsEnabled(logging.DEBUG) {
		req_copy := proto.Clone(req).(*crypto_proto.VeloMessage)
		req_copy.VQLClientAction = nil
		logger.Debug("Received request: %v", req_copy)
	}
}
