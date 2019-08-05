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
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/crypto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

type MockExecutor struct {
	to_send    []string
	sent_count int
	wg         sync.WaitGroup
}

func (self *MockExecutor) ReadFromServer() *crypto_proto.GrrMessage {
	return nil
}

func (self *MockExecutor) SendToServer(message *crypto_proto.GrrMessage)   {}
func (self *MockExecutor) ProcessRequest(message *crypto_proto.GrrMessage) {}
func (self *MockExecutor) ReadResponse() <-chan *crypto_proto.GrrMessage {
	result := make(chan *crypto_proto.GrrMessage)

	self.wg.Add(1)
	go func() {
		for _, item := range self.to_send {
			self.wg.Add(1)
			result <- &crypto_proto.GrrMessage{Name: item}
			self.sent_count++
		}
		self.wg.Done()
	}()

	return result
}

type MockHTTPConnector struct {
	wg        *sync.WaitGroup
	received  []string
	connected bool
}

func (self *MockHTTPConnector) GetCurrentUrl() string { return "http://URL/" }
func (self *MockHTTPConnector) Post(handler string, data []byte) (*http.Response, error) {
	// Emulate an error if we are not connected.
	if !self.connected {
		return &http.Response{
			Body:       ioutil.NopCloser(bytes.NewBufferString("")),
			StatusCode: 500,
		}, nil
	}

	manager := crypto.NullCryptoManager{}
	decrypted, _ := manager.DecryptMessageList(data)
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

func TestSender(t *testing.T) {
	config_obj, err := config.LoadConfig("test_data/client.config.yaml")
	assert.NoError(t, err)

	manager := &crypto.NullCryptoManager{}
	messages := []string{
		"0123456789",
		"0123456789",
		"0123456789"}
	exe := &MockExecutor{to_send: messages}
	logger := logging.GetLogger(config_obj, &logging.ClientComponent)

	enroller := &Enroller{
		config_obj: config_obj,
		manager:    manager,
		executor:   exe,
		logger:     logger}

	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)

	wg := &sync.WaitGroup{}
	wg.Add(1)
	connector := &MockHTTPConnector{wg: wg}
	go func() {
		select {
		case <-ctx.Done():
			wg.Done()
		}
	}()

	// The connector is not connected
	connector.connected = false

	sender := NewSender(
		config_obj, connector, manager, exe, enroller,
		logger, "Sender", "control")

	// Make the ring buffer 10 bytes - this is enough for one
	// message but no more.
	sender.ring_buffer.Size = 10
	sender.Start(ctx)

	// Wait until everything stabilizes
	time.Sleep(time.Second)

	// We only sent one message since there is no room in the
	// ring_buffer.
	assert.Equal(t, 1, exe.sent_count)

	// No messages are actually consumed.
	assert.Nil(t, connector.received)

	// Turn on the HTTP connector.
	connector.connected = true

	// Wait until all messages are sent.
	wg.Wait()
	assert.Equal(t, connector.received, messages)

	utils.Debug(connector.received)
}
