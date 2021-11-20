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
package http_comms

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	errors "github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	crypto_test "www.velocidex.com/golang/velociraptor/crypto/testing"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

type MockHTTPConnector struct {
	wg        *sync.WaitGroup
	mu        sync.Mutex
	received  []string
	connected bool
	t         *testing.T
}

func (self *MockHTTPConnector) GetCurrentUrl(handler string) string { return "http://URL/" + handler }
func (self *MockHTTPConnector) Post(handler string, data []byte, urgent bool) (*http.Response, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Emulate an error if we are not connected.
	if !self.connected {
		return nil, errors.New("Unavailable")
	}

	defer self.wg.Done()

	manager := crypto_test.NullCryptoManager{}

	message_info, err := manager.Decrypt(data)
	require.NoError(self.t, err)

	message_info.IterateJobs(context.Background(),
		func(ctx context.Context, item *crypto_proto.VeloMessage) {
			self.received = append(self.received, item.Name)
		})

	return &http.Response{
		Body:       ioutil.NopCloser(bytes.NewBufferString("")),
		StatusCode: 200,
	}, nil
}
func (self *MockHTTPConnector) ReKeyNextServer()   {}
func (self *MockHTTPConnector) ServerName() string { return "VelociraptorServer" }

// Try to send the message immediately. If we get through we increase the wg.
func CanSendToExecutor(
	wg *sync.WaitGroup,
	exec *executor.ClientExecutor,
	msg *crypto_proto.VeloMessage) bool {
	select {
	case exec.Outbound <- msg:
		// Add to the wg a task - this will be subtracted when
		// the final post is made.
		wg.Add(1)
		return true

	case <-time.After(500 * time.Millisecond):
		return false
	}
}

func testRingBuffer(
	rb IRingBuffer,
	config_obj *config_proto.Config,
	message string,
	t *testing.T) {
	t.Parallel()

	manager := &crypto_test.NullCryptoManager{}
	exe := &executor.ClientExecutor{
		Inbound:  make(chan *crypto_proto.VeloMessage),
		Outbound: make(chan *crypto_proto.VeloMessage),
	}
	logger := logging.GetLogger(config_obj, &logging.ClientComponent)

	// Set global timeout on the test.
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)

	wg := &sync.WaitGroup{}

	connector := &MockHTTPConnector{wg: wg, t: t}

	// The connector is not connected initially.
	connector.connected = false

	sender, err := NewSender(
		config_obj, connector, manager, exe, rb, nil, /* enroller */
		logger, "Sender", "control", nil, &utils.RealClock{})
	assert.NoError(t, err)

	sender.Start(ctx)

	// This message results in a 14 byte message enqueued. The
	// RingBuffer is allowed to go over the size slightly but will
	// prevent more messages from being generated. This ensures
	// that all messages will be written to disk in case of a
	// crash.
	msg := &crypto_proto.VeloMessage{
		Name: message,
	}

	// The first message will be enqueued.
	assert.Equal(t, CanSendToExecutor(wg, exe, msg), true)

	// These messages can not be sent since there is no room in
	// the buffer.
	assert.Equal(t, CanSendToExecutor(wg, exe, msg), false)

	// Nothing is received yet since the connector is
	// disconnected.
	assert.Equal(t, len(connector.received), 0)

	// The ring buffer is holding 14 bytes since none were
	// successfully sent yet.
	vtesting.WaitUntil(5*time.Second, t, func() bool {
		return sender.ring_buffer.AvailableBytes() == uint64(14)
	})

	// Turn the connector on - now sending will be successful. We
	// need to wait for the communicator to retry sending.
	connector.mu.Lock()
	connector.connected = true
	connector.mu.Unlock()

	// Wait until the messages are delivered.
	wg.Wait()

	// There is one message received
	assert.Equal(t, len(connector.received), 1)

	// Wait for the messages to be committed in the ring buffer.
	time.Sleep(500 * time.Millisecond)

	// The ring buffer is now truncated to 0.
	assert.Equal(t, sender.ring_buffer.AvailableBytes(), uint64(0))

	// We can send more messages.
	assert.Equal(t, CanSendToExecutor(wg, exe, msg), true)
}

func TestSender(t *testing.T) {
	config_obj := config.GetDefaultConfig()

	config_obj.Client.MaxPoll = 1
	config_obj.Client.MaxPollStd = 1

	// Make the ring buffer 10 bytes - this is enough for one
	// message but no more.
	rb := NewRingBuffer(config_obj, 10)
	testRingBuffer(rb, config_obj, "0123456789", t)
}

func TestSenderWithFileBuffer(t *testing.T) {
	config_obj := config.GetDefaultConfig()

	tmpfile, err := ioutil.TempFile("", "test")
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

	rb, err := NewFileBasedRingBuffer(config_obj, logger)
	require.NoError(t, err)

	testRingBuffer(rb, config_obj, "0123456789", t)
}
