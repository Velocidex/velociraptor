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

// Diagram to illustrate outgoing messages:
// executor -> ring buffer -> sender -> server

// The sender pushes messages through these channels:
// PumpExecutorToRingBuffer() -> PumpRingBufferToSendMessage() -> sendMessageList()

package http_comms

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-errors/errors"
	"golang.org/x/time/rate"
	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/crypto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

const (
	URGENT = true
)

type Sender struct {
	*NotificationReader

	// Signalled when a packet is full and should be sent
	// immediately - skip the minPoll wait when the queue is
	// already full.
	mu      sync.Mutex
	release chan bool

	ring_buffer IRingBuffer

	// An in-memory ring buffer for urgent packets.
	urgent_buffer *RingBuffer

	clock utils.Clock
}

func (self *Sender) CleanOnExit(ctx context.Context) {
	<-ctx.Done()
	self.urgent_buffer.Close()
	self.ring_buffer.Close()
}

// Persistent loop to pump messages from the executor to the ring
// buffer. This function should never exit in a real client.
func (self *Sender) PumpExecutorToRingBuffer(ctx context.Context) {

	// We should never exit from this.
	defer self.maybeCallOnExit()

	// Pump messages from the executor to the pending message list
	// - this is our local queue of output pending messages.
	executor_chan := self.executor.ReadResponse()

	for {
		if atomic.LoadInt32(&self.IsPaused) != 0 {
			select {
			case <-ctx.Done():
				return
			case <-self.clock.After(self.minPoll):
				executor.Nanny.UpdatePumpToRb()
				continue
			}
		}

		executor.Nanny.UpdatePumpToRb()

		select {
		case <-ctx.Done():
			return

			// Keep the nanny alive to ensure we are still inside this
			// loop.
		case <-time.After(self.maxPoll):
			continue

		case msg, ok := <-executor_chan:
			// Executor closed the channel.
			if !ok {
				return
			}

			if msg.Urgent {
				// Urgent messages are queued in
				// memory and dispatched separately.
				item := &crypto_proto.MessageList{
					Job: []*crypto_proto.VeloMessage{msg}}

				serialized_msg, err := proto.Marshal(item)
				if err != nil {
					// Can't serialize the message
					// - drop it on the floor.
					continue
				}
				self.urgent_buffer.Enqueue(serialized_msg)

			} else {
				// NOTE: This is kind of a hack. We hold in
				// memory a bunch of VeloMessage proto objects
				// and we want to serialize them into a
				// MessageList proto one at the time (so we
				// can track how large the final message is
				// going to be). We use the special wire
				// format property of protobufs that repeated
				// fields can be appended on the wire, and
				// then parsed as a single message. This saves
				// us encoding the VeloMessage just to see how
				// large it is going to be and then encoding
				// it again.
				item := &crypto_proto.MessageList{
					Job: []*crypto_proto.VeloMessage{msg}}
				serialized_msg, err := proto.Marshal(item)
				if err != nil {
					// Can't serialize the message
					// - drop it on the floor.
					continue
				}

				// RingBuffer.Enqueue may block if there is
				// no room in the ring buffer. While waiting
				// here we block the executor channel.
				self.ring_buffer.Enqueue(serialized_msg)
			}

			// We have just filled the message queue with
			// enough data, trigger the sender to send
			// this data out immediately.
			if self.ring_buffer.AvailableBytes() > self.
				config_obj.Client.MaxUploadSize {

				// Signal to
				// PumpRingBufferToSendMessage() that
				// it should not wait before sending
				// the next packet.
				self.mu.Lock()
				close(self.release)
				self.release = make(chan bool)
				self.mu.Unlock()
			}
		}
	}
}

// Manages the sending of messages to the server. Reads messages from
// the ring buffer if there are any to send and compose a Message List
// to send. This also manages timing and retransmissions - blocks if
// the server is not available. This function should never exit in a
// real client.
func (self *Sender) PumpRingBufferToSendMessage(ctx context.Context) {

	compression := crypto_proto.PackedMessageList_ZCOMPRESSION
	if self.config_obj.Client.DisableCompression {
		compression = crypto_proto.PackedMessageList_UNCOMPRESSED
	}

	for {
		executor.Nanny.UpdatePumpRbToServer()

		if atomic.LoadInt32(&self.IsPaused) == 0 {
			// Grab some messages from the urgent ring buffer.
			compressed_messages := LeaseAndCompress(self.urgent_buffer,
				self.config_obj.Client.MaxUploadSize, compression)
			if len(compressed_messages) > 0 {
				self.sendMessageList(ctx, compressed_messages, URGENT, compression)
				if utils.IsCtxDone(ctx) {
					self.ring_buffer.Rollback()
				} else {
					self.ring_buffer.Commit()
				}
			}

			// Grab some messages from the ring buffer.
			compressed_messages = LeaseAndCompress(self.ring_buffer,
				self.config_obj.Client.MaxUploadSize, compression)
			if len(compressed_messages) > 0 {
				// sendMessageList will block until the messages are
				// successfully sent to the server. When it returns we
				// know the messages are sent so we can commit them
				// from the ring buffer.
				self.sendMessageList(ctx, compressed_messages, !URGENT, compression)
				if utils.IsCtxDone(ctx) {
					self.ring_buffer.Rollback()
				} else {
					self.ring_buffer.Commit()
				}
			}
		}

		self.mu.Lock()
		release := self.release
		self.mu.Unlock()

		// Wait a minimum time before sending the next one to
		// give the executor a chance to fill the queue.
		select {
		case <-ctx.Done():
			return

			// If the queue is too large we need to flush
			// it out immediately so skip the wait below.
		case <-release:
			continue

			// Wait a minimum amount of time to allow for
			// responses to be queued in the same POST.
		case <-self.clock.After(self.minPoll):
			continue
		}
	}
}

// The sender simply sends any server bound messages to the server. We
// only send messages when responses are pending.
func (self *Sender) Start(
	ctx context.Context, wg *sync.WaitGroup) {

	wg.Add(1)
	go func() {
		defer wg.Done()
		self.PumpExecutorToRingBuffer(ctx)

	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		// We should never exit from this.
		defer self.maybeCallOnExit()

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Allow cancellation of the main loop. This will restart
			// it as needed.
			sub_ctx, cancel := context.WithCancel(ctx)
			self.cancel = cancel

			self.PumpRingBufferToSendMessage(sub_ctx)
			cancel()
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		self.CleanOnExit(ctx)
	}()
}

func NewSender(
	config_obj *config_proto.Config,
	connector IConnector,
	crypto_manager crypto.ICryptoManager,
	executor executor.Executor,
	ring_buffer IRingBuffer,
	enroller *Enroller,
	logger *logging.LogContext,
	name string,
	limiter *rate.Limiter,
	handler string,
	on_exit func(),
	clock utils.Clock) (*Sender, error) {

	if config_obj.Client == nil {
		return nil, errors.New("Client not configured")
	}

	result := &Sender{
		NotificationReader: NewNotificationReader(
			config_obj, connector, crypto_manager,
			executor, enroller, logger, name,
			limiter, handler, on_exit, clock),
		ring_buffer: ring_buffer,

		// Urgent buffer is an in memory ring buffer to handle
		// urgent queries. This ensures urgent queries can
		// skip the buffer ahead of normal queries.
		urgent_buffer: NewRingBuffer(config_obj, executor.FlowManager(),
			2*config_obj.Client.MaxUploadSize, "UrgentBuffer"),
		release: make(chan bool),
		clock:   clock,
	}

	executor.Nanny().RegisterOnWarnings(result.id, func() {
		result.logger.Info(
			"<red>%s: Nanny issued first warning!</> Restarting Sender comms",
			result.name)
		result.Restart()
	})

	return result, nil
}
