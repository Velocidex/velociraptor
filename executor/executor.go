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
	"time"

	"www.velocidex.com/golang/velociraptor/actions"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/utils"

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

// Keep track of cancelled flows client side. NOTE We never expire the
// map of cancelled flows but it is expected to be very uncommon.
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

// _FlowContext keeps track of all the queries running as part of a
// given flow. When the flow is cancelled we cancel all these queries.
type _FlowContext struct {
	cancel  func()
	id      int
	flow_id string
}

type ClientExecutor struct {
	client_id string

	Inbound  chan *crypto_proto.VeloMessage
	Outbound chan *crypto_proto.VeloMessage

	// Map all the contexts with the flow id.
	mu         sync.Mutex
	config_obj *config_proto.Config
	in_flight  map[string][]*_FlowContext
	next_id    int

	concurrency *utils.Concurrency
}

func (self *ClientExecutor) ClientId() string {
	return self.client_id
}

func (self *ClientExecutor) Cancel(
	ctx context.Context, flow_id string, responder *responder.Responder) bool {
	if Canceller.IsCancelled(flow_id) {
		return false
	}

	self.mu.Lock()
	contexts, ok := self.in_flight[flow_id]
	if ok {
		contexts = contexts[:]
	}
	self.mu.Unlock()

	if ok {
		// Cancel all existing queries.
		Canceller.Cancel(flow_id)
		for _, flow_ctx := range contexts {
			flow_ctx.cancel()
		}

		return true
	}

	return ok
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
// Note: There are multiple queries tied to the same flow id but all
// of them need to be cancelled when the flow is cancelled.
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

	// Handle the requests. This used to be a plugin registration
	// process but there are very few plugins any more and so it
	// is easier to hard code this.
	responder := responder.NewResponder(config_obj, req, self.Outbound)

	if req.VQLClientAction != nil {
		// Control concurrency on the executor only.
		if !req.Urgent {
			cancel, err := self.concurrency.StartConcurrencyControl(ctx)
			if err != nil {
				responder.RaiseError(ctx, fmt.Sprintf("%v", err))
				return
			}
			defer cancel()
		}
		actions.VQLClientAction{}.StartQuery(
			config_obj, ctx, responder, req.VQLClientAction)
		return
	}

	if req.UpdateEventTable != nil {
		actions.UpdateEventTable{}.Run(
			config_obj, ctx, responder, req.UpdateEventTable)
		return
	}

	// This action is deprecated now.
	if req.UpdateForeman != nil {
		return
	}

	if req.Cancel != nil {
		// Only log when the flow is not already cancelled.
		if self.Cancel(ctx, req.SessionId, responder) {
			makeErrorResponse(self.Outbound,
				req, fmt.Sprintf("Cancelled all inflight queries for flow %v",
					req.SessionId))
		}
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
		in_flight:   make(map[string][]*_FlowContext),
		config_obj:  config_obj,
		concurrency: utils.NewConcurrencyControl(level, time.Hour),
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
					// Each request has its own context.
					ctx, flow_context := result._FlowContext(
						req.SessionId)
					logger.Debug("Received request: %v", req)

					wg.Add(1)
					go func() {
						defer wg.Done()

						result.processRequestPlugin(
							config_obj, ctx, req)
						result._CloseContext(flow_context)
					}()
				}
			}
		}
	}()

	return result, nil
}
