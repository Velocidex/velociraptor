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
	"sync"

	"www.velocidex.com/golang/velociraptor/actions"
	"www.velocidex.com/golang/velociraptor/constants"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/responder"
)

var (
	Canceller = &canceller{
		cancelled: make(map[string]bool),
	}
)

// Keep track of cancelled flows client side.
type canceller struct {
	mu        sync.Mutex
	cancelled map[string]bool
}

func (self *canceller) Cancel(flow_id string) {
	// Some flows are non-cancellable.
	switch flow_id {
	case constants.MONITORING_WELL_KNOWN_FLOW:
		return
	}

	self.mu.Lock()
	self.cancelled[flow_id] = true
	self.mu.Unlock()
}

func (self *canceller) IsCancelled(flow_id string) bool {
	self.mu.Lock()
	_, pres := self.cancelled[flow_id]
	self.mu.Unlock()

	return pres
}

type Executor interface {
	// These are called by the executor code.
	ReadFromServer() *crypto_proto.GrrMessage
	SendToServer(message *crypto_proto.GrrMessage)

	// These two are called by the comms module.

	// Feed a server request to the executor for execution.
	ProcessRequest(
		ctx context.Context,
		message *crypto_proto.GrrMessage)

	// Read a single response from the executor to be sent to the server.
	ReadResponse() <-chan *crypto_proto.GrrMessage
}

// A concerete implementation of a client executor.

// _FlowContext keeps track of all the queries running as part of a
// given flow. When the flow is cancelled we cancel all these queries.
type _FlowContext struct {
	cancel  func()
	id      int
	flow_id string
}

type ClientExecutor struct {
	Inbound  chan *crypto_proto.GrrMessage
	Outbound chan *crypto_proto.GrrMessage

	// Map all the contexts with the flow id.
	mu         sync.Mutex
	config_obj *config_proto.Config
	in_flight  map[string][]*_FlowContext
	next_id    int
}

func (self *ClientExecutor) Cancel(flow_id string, responder *responder.Responder) {
	self.mu.Lock()
	defer self.mu.Unlock()

	contexts, ok := self.in_flight[flow_id]
	if ok {
		responder.Log("Cancelling %v in flight queries", len(contexts))
		for _, flow_ctx := range contexts {
			flow_ctx.cancel()
		}

		Canceller.Cancel(flow_id)
	}
}

func (self *ClientExecutor) _FlowContext(flow_id string) (context.Context, *_FlowContext) {
	self.mu.Lock()
	defer self.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())

	result := &_FlowContext{
		flow_id: flow_id,
		cancel:  cancel,
		id:      self.next_id,
	}
	self.next_id++

	contexts, ok := self.in_flight[flow_id]
	if ok {
		contexts = append(contexts, result)
	} else {
		contexts = []*_FlowContext{result}
	}
	self.in_flight[flow_id] = contexts

	return ctx, result
}

// _CloseContext removes the flow_context from the in_flight map.
func (self *ClientExecutor) _CloseContext(flow_context *_FlowContext) {
	self.mu.Lock()
	defer self.mu.Unlock()

	contexts, ok := self.in_flight[flow_context.flow_id]
	if ok {
		new_context := make([]*_FlowContext, 0, len(contexts))
		for i := 0; i < len(contexts); i++ {
			if contexts[i].id != flow_context.id {
				new_context = append(new_context, contexts[i])
			}
		}

		if len(new_context) == 0 {
			delete(self.in_flight, flow_context.flow_id)
		} else {
			self.in_flight[flow_context.flow_id] = new_context
		}
	}
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

func (self *ClientExecutor) ProcessRequest(
	ctx context.Context,
	message *crypto_proto.GrrMessage) {
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
		actions.VQLClientAction{}.StartQuery(
			config_obj, ctx, responder, req.VQLClientAction)
		return
	}

	if req.UpdateEventTable != nil {
		actions.UpdateEventTable{}.Run(
			config_obj, ctx, responder, req.UpdateEventTable)
		return
	}

	if req.UpdateForeman != nil {
		actions.UpdateForeman{}.Run(
			config_obj, ctx, responder, req.UpdateForeman)
		return
	}

	if req.Cancel != nil {
		self.Cancel(req.SessionId, responder)
		self.Outbound <- makeErrorResponse(
			req, fmt.Sprintf("Cancelled all inflight queries: %v",
				req.SessionId))
		return
	}

	self.Outbound <- makeErrorResponse(
		req, fmt.Sprintf("Unsupported payload for message: %v", req))
}

func NewClientExecutor(config_obj *config_proto.Config) (*ClientExecutor, error) {
	result := &ClientExecutor{
		Inbound:    make(chan *crypto_proto.GrrMessage),
		Outbound:   make(chan *crypto_proto.GrrMessage),
		in_flight:  make(map[string][]*_FlowContext),
		config_obj: config_obj,
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
				ctx, flow_context := result._FlowContext(req.SessionId)
				logger.Info("Received request: %v", req)

				go func() {
					result.processRequestPlugin(config_obj, ctx, req)
					result._CloseContext(flow_context)
				}()
			}
		}
	}()

	return result, nil
}
