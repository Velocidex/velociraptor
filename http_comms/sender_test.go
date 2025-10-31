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
package http_comms

import (
	"bytes"
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/go-errors/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	crypto_test "www.velocidex.com/golang/velociraptor/crypto/testing"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

type MockHTTPConnector struct {
	wg        *sync.WaitGroup
	mu        sync.Mutex
	received  []string
	connected bool
	t         *testing.T

	config_obj *config_proto.Config
}

func (self *MockHTTPConnector) GetCurrentUrl(handler string) string {
	return "http://URL/" + handler
}

func (self *MockHTTPConnector) SetConnected(c bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.connected = c
}

func (self *MockHTTPConnector) Connected() bool {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.connected
}

func (self *MockHTTPConnector) Post(ctx context.Context,
	name, handler string, data []byte, urgent bool) (*bytes.Buffer, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Emulate an error if we are not connected.
	if !self.connected {
		return nil, errors.New("Unavailable")
	}

	// Decreasse the wg when the message arrives.
	defer self.wg.Done()

	manager := crypto_test.NullCryptoManager{}

	message_info, err := manager.Decrypt(ctx, data)
	require.NoError(self.t, err)

	message_info.IterateJobs(context.Background(), self.config_obj,
		func(ctx context.Context, item *crypto_proto.VeloMessage) error {
			self.received = append(self.received, item.Name)
			return nil
		})

	return &bytes.Buffer{}, nil
}

func (self *MockHTTPConnector) ReKeyNextServer(ctx context.Context) {}

func (self *MockHTTPConnector) ServerName() string {
	return utils.GetSuperuserName(self.config_obj)
}

// Try to send the message immediately.
func CanSendToExecutor(
	exec *executor.ClientExecutor,
	msg *crypto_proto.VeloMessage) bool {
	select {
	case exec.Outbound <- msg:
		return true

	case <-time.After(500 * time.Millisecond):
		return false
	}
}

func testRingBuffer(
	ctx context.Context,
	rb IRingBuffer,
	config_obj *config_proto.Config,
	message string,
	t *testing.T) {

	t.Parallel()

	wg := &sync.WaitGroup{}

	manager := &crypto_test.NullCryptoManager{}
	exe := &executor.ClientExecutor{
		Inbound:  make(chan *crypto_proto.VeloMessage),
		Outbound: make(chan *crypto_proto.VeloMessage),
	}
	logger := logging.GetLogger(config_obj, &logging.ClientComponent)

	// Set global timeout on the test.
	subctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer func() {
		cancel()
		wg.Wait()
	}()

	// We use this to wait for messages to be delivered to the mock
	mock_wg := &sync.WaitGroup{}

	connector := &MockHTTPConnector{
		config_obj: config_obj,
		wg:         mock_wg,
		t:          t}

	// The connector is not connected initially.
	connector.SetConnected(false)

	sender, err := NewSender(
		config_obj, connector, manager, exe, rb, nil, /* enroller */
		logger, "Sender", rate.NewLimiter(rate.Inf, 0),
		"control", nil, &utils.RealClock{})
	assert.NoError(t, err)

	sender.Start(subctx, wg)

	// This message results in a 14 byte message enqueued. The
	// RingBuffer is allowed to go over the size slightly but will
	// prevent more messages from being generated. This ensures
	// that all messages will be written to disk in case of a
	// crash.
	msg := &crypto_proto.VeloMessage{
		Name: message,
	}

	// The first message will be enqueued.
	mock_wg.Add(1) // Wait for it to be finally delivered
	assert.Equal(t, CanSendToExecutor(exe, msg), true)

	// These messages can not be sent since there is no room in
	// the buffer.
	assert.Equal(t, CanSendToExecutor(exe, msg), false)

	// Nothing is received yet since the connector is
	// disconnected.
	assert.Equal(t, len(connector.received), 0)

	// The ring buffer is holding 14 bytes since none were
	// successfully sent yet.
	vtesting.WaitUntil(10*time.Second, t, func() bool {
		return sender.ring_buffer.TotalSize() == uint64(14)
	})

	// Turn the connector on - now sending will be successful. We
	// need to wait for the communicator to retry sending.
	connector.SetConnected(true)

	// Wait until the messages are delivered.
	mock_wg.Wait()

	// There is one message received
	assert.Equal(t, len(connector.received), 1)

	// Wait for the messages to be committed in the ring buffer.
	vtesting.WaitUntil(time.Second, t, func() bool {
		// The ring buffer is now truncated to 0.
		return sender.ring_buffer.TotalSize() == 0
	})

	// We can send more messages.
	assert.Equal(t, CanSendToExecutor(exe, msg), true)
}

func TestSender(t *testing.T) {
	config_obj := config.GetDefaultConfig()

	config_obj.Client.MaxPoll = 1
	config_obj.Client.MaxPollStd = 1

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Make the ring buffer 10 bytes - this is enough for one
	// message but no more.
	flow_manager := responder.NewFlowManager(ctx, config_obj, "")
	rb := NewRingBuffer(config_obj, flow_manager, 10, "Sender")
	testRingBuffer(ctx, rb, config_obj, "0123456789", t)
}

func TestSenderWithFileBuffer(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	tmpfile, err := tempfile.TempFile("test")
	require.NoError(t, err)
	defer os.Remove(tmpfile.Name())

	tmpfile.Close()

	// Make the ring buffer 10 bytes - this is enough for one
	// message but no more.
	config_obj.Client.LocalBuffer.DiskSize = 10
	config_obj.Client.LocalBuffer.FilenameLinux = tmpfile.Name()
	config_obj.Client.LocalBuffer.FilenameWindows = tmpfile.Name()
	config_obj.Client.LocalBuffer.FilenameDarwin = tmpfile.Name()
	config_obj.Client.MaxPoll = 1
	config_obj.Client.MaxPollStd = 1

	logger := logging.GetLogger(config_obj, &logging.ClientComponent)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	flow_manager := responder.NewFlowManager(ctx, config_obj, "")
	local_buffer_name := getLocalBufferName(config_obj)
	rb, err := NewFileBasedRingBuffer(ctx, config_obj,
		local_buffer_name, flow_manager, logger)
	require.NoError(t, err)

	testRingBuffer(ctx, rb, config_obj, "0123456789", t)
}
