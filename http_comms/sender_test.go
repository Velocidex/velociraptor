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
	to_send []*crypto_proto.GrrMessage
	idx     int
	wg      *sync.WaitGroup
}

func (self *MockExecutor) ReadFromServer() *crypto_proto.GrrMessage {
	return nil
}

func (self *MockExecutor) SendToServer(message *crypto_proto.GrrMessage)   {}
func (self *MockExecutor) ProcessRequest(message *crypto_proto.GrrMessage) {}
func (self *MockExecutor) ReadResponse() <-chan *crypto_proto.GrrMessage {
	result := make(chan *crypto_proto.GrrMessage)

	go func() {
		for _, item := range self.to_send {
			wg.Add(1)
			result <- item
		}
	}()

	return result
}

type MockHTTPConnector struct {
	wg *sync.WaitGroup
}

func (self *MockHTTPConnector) GetCurrentUrl() string { return "http://URL/" }
func (self *MockHTTPConnector) Post(handler string, data []byte) (*http.Response, error) {
	utils.Debug(data)
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

	server_config_obj, err := config.LoadConfig("test_data/server.config.yaml")
	assert.NoError(t, err)

	private_key, err := crypto.GeneratePrivateKey()
	assert.NoError(t, err)

	manager, err := crypto.NewClientCryptoManager(config_obj, private_key)
	assert.NoError(t, err)

	manager.AddCertificate([]byte(server_config_obj.Frontend.Certificate))

	exe := &MockExecutor{to_send: []*crypto_proto.GrrMessage{
		&crypto_proto.GrrMessage{},
	}}
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

	sender := NewSender(
		config_obj, connector, manager, exe, enroller,
		logger, "Sender", "control")
	sender.Start(ctx)
	wg.Wait()
}
