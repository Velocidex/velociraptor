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
//
package server

import (
	"context"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	errors "github.com/pkg/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/crypto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/flows"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/urns"
)

type NotificationPool struct {
	mu      sync.Mutex
	clients map[string]chan bool
}

func NewNotificationPool() *NotificationPool {
	return &NotificationPool{
		clients: make(map[string]chan bool),
	}
}

func (self *NotificationPool) Listen(client_id string) (chan bool, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Close any old channels and make a new one.
	c, pres := self.clients[client_id]
	if pres {
		// This could happen because the client was connected,
		// but the connection is dropped and the HTTP receiver
		// is still blocked. This unblocks the old connection
		// and returns an error on the new connection at the
		// same time. If the old client is still connected, it
		// will reconnect immediately but the new client will
		// wait the full max poll to retry.
		close(c)
		delete(self.clients, client_id)

		return nil, errors.New("Only one listener may exist.")
	}

	c = make(chan bool)
	self.clients[client_id] = c

	return c, nil
}

func (self *NotificationPool) Notify(client_id string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	c, pres := self.clients[client_id]
	if pres {
		close(c)
		delete(self.clients, client_id)
	}
}

func (self *NotificationPool) Shutdown() {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Send all the readers the quit signal and shut down the
	// pool.
	for _, c := range self.clients {
		c <- true
		close(c)
	}

	self.clients = make(map[string]chan bool)
}

func (self *NotificationPool) NotifyAll() {
	self.mu.Lock()
	defer self.mu.Unlock()

	for _, c := range self.clients {
		close(c)
	}

	self.clients = make(map[string]chan bool)
}

type Server struct {
	config           *api_proto.Config
	manager          *crypto.CryptoManager
	logger           *logging.LogContext
	db               datastore.DataStore
	NotificationPool *NotificationPool

	// Limit concurrency for processing messages.
	concurrency chan bool
}

func (self *Server) StartConcurrencyControl() {
	// Wait here until we have enough room in the concurrency
	// channel.
	self.concurrency <- true
}

func (self *Server) EndConcurrencyControl() {
	<-self.concurrency
}

func (self *Server) Close() {
	self.db.Close()
	self.NotificationPool.Shutdown()
}

func NewServer(config_obj *api_proto.Config) (*Server, error) {
	manager, err := crypto.NewServerCryptoManager(config_obj)
	if err != nil {
		return nil, err
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	concurrency := config_obj.Frontend.Concurrency
	if concurrency == 0 {
		concurrency = 50
	}

	result := Server{
		config:           config_obj,
		manager:          manager,
		db:               db,
		NotificationPool: NewNotificationPool(),
		logger: logging.GetLogger(config_obj,
			&logging.FrontendComponent),
		concurrency: make(chan bool, concurrency),
	}
	return &result, nil
}

// Only process messages from the Velociraptor client.
func (self *Server) processVelociraptorMessages(
	ctx context.Context,
	client_id string,
	messages []*crypto_proto.GrrMessage) error {

	runner := flows.NewFlowRunner(self.config, self.logger)
	defer runner.Close()

	runner.ProcessMessages(messages)

	return nil
}

// We only process some messages when the client is not authenticated.
func (self *Server) ProcessUnauthenticatedMessages(
	ctx context.Context,
	message_info *crypto.MessageInfo) error {
	message_list := &crypto_proto.MessageList{}
	err := proto.Unmarshal(message_info.Raw, message_list)
	if err != nil {
		return errors.WithStack(err)
	}

	for _, message := range message_list.Job {
		switch message.SessionId {

		case "aff4:/flows/E:Enrol":
			err := enroll(self, message)
			if err != nil {
				self.logger.Error("Enrol Error: %s", err)
			}
		}
	}

	return nil
}

func (self *Server) Decrypt(ctx context.Context, request []byte) (
	*crypto.MessageInfo, error) {
	message_info, err := self.manager.Decrypt(request)
	if err != nil {
		return nil, err
	}

	return message_info, nil
}

func (self *Server) Process(
	ctx context.Context,
	message_info *crypto.MessageInfo,
	drain_requests_for_client bool) (
	[]byte, int, error) {
	message_list := &crypto_proto.MessageList{}
	err := proto.Unmarshal(message_info.Raw, message_list)
	if err != nil {
		return nil, 0, errors.WithStack(err)
	}

	var messages []*crypto_proto.GrrMessage
	for _, message := range message_list.Job {
		if message_info.Authenticated {
			message.AuthState = crypto_proto.GrrMessage_AUTHENTICATED
		}
		message.Source = message_info.Source
		messages = append(messages, message)
	}

	err = self.processVelociraptorMessages(
		ctx, message_info.Source, messages)
	if err != nil {
		return nil, 0, err
	}

	// Record some stats about the client.
	client_info := &actions_proto.ClientInfo{
		Ping:      uint64(time.Now().UTC().UnixNano() / 1000),
		IpAddress: message_info.RemoteAddr,
	}

	err = self.db.SetSubject(
		self.config, urns.BuildURN("clients",
			message_info.Source, "ping"),
		client_info)
	if err != nil {
		return nil, 0, err
	}

	message_list = &crypto_proto.MessageList{}
	if drain_requests_for_client {
		message_list.Job = append(
			message_list.Job,
			self.DrainRequestsForClient(message_info.Source)...)
	}

	response, err := self.manager.EncryptMessageList(
		message_list, message_info.Source)
	if err != nil {
		return nil, 0, err
	}

	return response, len(message_list.Job), nil
}

func (self *Server) DrainRequestsForClient(client_id string) []*crypto_proto.GrrMessage {
	result, err := self.db.GetClientTasks(self.config, client_id, false)
	if err == nil {
		return result
	}

	return []*crypto_proto.GrrMessage{}
}

func (self *Server) Error(msg string, err error) {
	self.logger.Error(msg, err)
}

func (self *Server) Info(format string, v ...interface{}) {
	self.logger.Info(format, v...)
}
