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
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/crypto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/logging"
)

type MockHTTPConnector struct {
	wg        *sync.WaitGroup
	received  []string
	connected bool
}

func (self *MockHTTPConnector) GetCurrentUrl() string { return "http://URL/" }
func (self *MockHTTPConnector) Post(handler string, data []byte) (*http.Response, error) {
	// Emulate an error if we are not connected.
	if !self.connected {
		return nil, errors.New("Unavailable")
	}

	manager := crypto.NullCryptoManager{}
	decrypted, err := manager.DecryptMessageList(data)
	if err != nil {
		panic(err)
	}

	for _, item := range decrypted.Job {
		self.received = append(self.received, item.Name)
	}

	self.wg.Done()
	return &http.Response{
		Body:       ioutil.NopCloser(bytes.NewBufferString("")),
		StatusCode: 200,
	}, nil
}
func (self *MockHTTPConnector) ReKeyNextServer()   {}
func (self *MockHTTPConnector) ServerName() string { return "VelociraptorServer" }

// Try to send the message immediately.
func CanSendToExecutor(
	wg *sync.WaitGroup,
	exec *executor.ClientExecutor,
	msg *crypto_proto.GrrMessage) bool {
	select {
	case exec.Outbound <- msg:
		wg.Add(1)
		return true
	case <-time.After(500 * time.Millisecond):
		return false
	}
}

func TestSender(t *testing.T) {
	config_obj := config.GetDefaultConfig()

	// Make the ring buffer 10 bytes - this is enough for one
	// message but no more.
	config_obj.Client.LocalBuffer.MemorySize = 10
	config_obj.Client.MaxPoll = 1
	config_obj.Client.MaxPollStd = 1

	manager := &crypto.NullCryptoManager{}
	exe := &executor.ClientExecutor{
		Inbound:  make(chan *crypto_proto.GrrMessage),
		Outbound: make(chan *crypto_proto.GrrMessage),
	}
	logger := logging.GetLogger(config_obj, &logging.ClientComponent)

	// Set global timeout on the test.
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)

	wg := &sync.WaitGroup{}

	connector := &MockHTTPConnector{wg: wg}

	// The connector is not connected initially.
	connector.connected = false

	rb := NewRingBuffer(config_obj)
	sender := NewSender(
		config_obj, connector, manager, exe, rb, nil, /* enroller */
		logger, "Sender", "control")
	sender.Start(ctx)

	// This message results in a 14 byte message enqueued. The
	// RingBuffer is allowed to go over the size slightly but will
	// prevent more messages from being generated. This ensures
	// that all messages will be written to disk in case of a
	// crash.
	msg := &crypto_proto.GrrMessage{
		Name: "0123456789",
	}

	// The first message will be enqueued.
	assert.Equal(t, CanSendToExecutor(wg, exe, msg), true)

	// These messages can not be sent since there is no room in
	// the buffer.
	assert.Equal(t, CanSendToExecutor(wg, exe, msg), false)
	assert.Equal(t, CanSendToExecutor(wg, exe, msg), false)

	// Nothing is received yet since the connector is
	// disconnected.
	assert.Equal(t, len(connector.received), 0)

	// The ring buffer is holding 14 bytes since none were
	// successfully sent yet.
	assert.Equal(t, sender.ring_buffer.AvailableBytes(), uint64(14))

	// Turn the connector on - now sending will be successful. We
	// need to wait for the communicator to retry sending.
	connector.connected = true

	// Wait until the messages are delivered.
	wg.Wait()

	// There is one message received
	assert.Equal(t, len(connector.received), 1)

	// The ring buffer is now truncated to 0.
	assert.Equal(t, sender.ring_buffer.AvailableBytes(), uint64(0))

	// We can send more messages.
	assert.Equal(t, CanSendToExecutor(wg, exe, msg), true)
}

func TestSenderWithFileBuffer(t *testing.T) {
	config_obj := config.GetDefaultConfig()

	// Make the ring buffer 10 bytes - this is enough for one
	// message but no more.
	os.Remove("/tmp/1.bin")

	config_obj.Client.LocalBuffer.DiskSize = 10
	config_obj.Client.LocalBuffer.Filename = "/tmp/1.bin"
	config_obj.Client.MaxPoll = 1
	config_obj.Client.MaxPollStd = 1

	manager := &crypto.NullCryptoManager{}
	exe := &executor.ClientExecutor{
		Inbound:  make(chan *crypto_proto.GrrMessage),
		Outbound: make(chan *crypto_proto.GrrMessage),
	}
	logger := logging.GetLogger(config_obj, &logging.ClientComponent)

	// Set global timeout on the test.
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)

	wg := &sync.WaitGroup{}

	connector := &MockHTTPConnector{wg: wg}

	// The connector is not connected initially.
	connector.connected = false

	rb, err := NewFileBasedRingBuffer(config_obj)
	assert.NoError(t, err)

	sender := NewSender(
		config_obj, connector, manager, exe, rb, nil, /* enroller */
		logger, "Sender", "control")
	sender.Start(ctx)

	// This message results in a 14 byte message enqueued. The
	// RingBuffer is allowed to go over the size slightly but will
	// prevent more messages from being generated. This ensures
	// that all messages will be written to disk in case of a
	// crash.
	msg := &crypto_proto.GrrMessage{
		Name: "0123456789",
	}

	// The first message will be enqueued.
	assert.Equal(t, CanSendToExecutor(wg, exe, msg), true)

	// These messages can not be sent since there is no room in
	// the buffer.
	assert.Equal(t, CanSendToExecutor(wg, exe, msg), false)
	assert.Equal(t, CanSendToExecutor(wg, exe, msg), false)

	// Nothing is received yet since the connector is
	// disconnected.
	assert.Equal(t, len(connector.received), 0)

	// The ring buffer is holding 14 bytes since none were
	// successfully sent yet.
	assert.Equal(t, sender.ring_buffer.AvailableBytes(), uint64(14))

	// Turn the connector on - now sending will be successful. We
	// need to wait for the communicator to retry sending.
	connector.connected = true

	// Wait until the messages are delivered.
	wg.Wait()

	// There is one message received
	assert.Equal(t, len(connector.received), 1)

	// The ring buffer is now truncated to 0.
	assert.Equal(t, sender.ring_buffer.AvailableBytes(), uint64(0))

	// We can send more messages.
	assert.Equal(t, CanSendToExecutor(wg, exe, msg), true)
}
