//
package server

import (
	"context"
	"errors"
	"github.com/golang/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/crypto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/flows"
	"www.velocidex.com/golang/velociraptor/logging"
	//	utils "www.velocidex.com/golang/velociraptor/testing"
)

type Server struct {
	config  *config.Config
	manager *crypto.CryptoManager
	logger  *logging.Logger
	db      datastore.DataStore
}

func (self *Server) Close() {
	self.db.Close()
}

func NewServer(config_obj *config.Config) (*Server, error) {
	manager, err := crypto.NewServerCryptoManager(config_obj)
	if err != nil {
		return nil, err
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	result := Server{
		config:  config_obj,
		manager: manager,
		db:      db,
		logger:  logging.NewLogger(config_obj),
	}
	return &result, nil
}

// Only process messages from the Velociraptor client.
func (self *Server) processVelociraptorMessages(
	ctx context.Context,
	client_id string,
	messages []*crypto_proto.GrrMessage) error {

	runner := flows.NewFlowRunner(self.config, self.db)
	defer runner.Close()
	runner.ProcessMessages(messages)

	var tasks_to_remove []uint64

	// Remove the well known flows and keep the other
	// messages for processing.
	for _, message := range messages {
		// Velociraptor clients always return their task id so
		// we can dequeue their messages immediately.
		if message.TaskId != nil &&
			message.Type != nil &&
			*message.Type == crypto_proto.GrrMessage_STATUS {
			tasks_to_remove = append(tasks_to_remove, *message.TaskId)
		}
	}

	// Remove outstanding tasks.
	self.db.RemoveTasksFromClientQueue(self.config, client_id, tasks_to_remove)

	return nil
}

func (self *Server) processGRRMessages(
	ctx context.Context,
	client_id string,
	messages []*crypto_proto.GrrMessage) error {
	return nil
}

// We only process some messages when the client is not authenticated.
func (self *Server) processUnauthenticatedMessages(
	ctx context.Context,
	client_id string,
	messages *crypto_proto.MessageList) error {

	for _, message := range messages.Job {
		switch *message.SessionId {

		case "aff4:/flows/E:Enrol":
			err := enroll(self, message)
			if err != nil {
				self.logger.Error("Enrol Error: %s", err)
			}
		}
	}

	return nil
}

func (self *Server) Process(ctx context.Context, request []byte) ([]byte, error) {
	message_info, err := self.manager.Decrypt(request)
	if err != nil {
		return nil, err
	}

	client_id := message_info.Source

	message_list := &crypto_proto.MessageList{}
	err = proto.Unmarshal(message_info.Raw, message_list)
	if err != nil {
		return nil, err
	}

	if !message_info.Authenticated {
		self.processUnauthenticatedMessages(ctx, *client_id, message_list)
		return nil, errors.New("Enrolment")
	}

	// Here we split incoming messages from Velociraptor clients
	// and GRR clients. We process the Velociraptor clients
	// ourselves, while relaying the GRR client's messages to the
	// GRR frontend server.
	var grr_messages []*crypto_proto.GrrMessage
	var velociraptor_messages []*crypto_proto.GrrMessage

	for _, message := range message_list.Job {
		if message_info.Authenticated {
			auth := crypto_proto.GrrMessage_AUTHENTICATED
			message.AuthState = &auth
		}
		message.Source = client_id
		if message.ClientType != nil &&
			*message.ClientType == crypto_proto.GrrMessage_VELOCIRAPTOR {
			velociraptor_messages = append(velociraptor_messages, message)
		} else {
			grr_messages = append(grr_messages, message)
		}
	}

	err = self.processVelociraptorMessages(ctx, *client_id, velociraptor_messages)
	if err != nil {
		return nil, err
	}

	err = self.processGRRMessages(ctx, *client_id, grr_messages)
	if err != nil {
		return nil, err
	}

	message_list = &crypto_proto.MessageList{}
	for _, response := range self.DrainRequestsForClient(*client_id) {
		message_list.Job = append(message_list.Job, response)
	}

	response, err := self.manager.EncryptMessageList(
		message_list, *message_info.Source)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func (self *Server) DrainRequestsForClient(client_id string) []*crypto_proto.GrrMessage {
	result, err := self.db.GetClientTasks(self.config, client_id)
	if err == nil {
		return result
	}

	return []*crypto_proto.GrrMessage{}
}

func (self *Server) Error(format string, v ...interface{}) {
	self.logger.Error(format, v...)
}

func (self *Server) Info(format string, v ...interface{}) {
	self.logger.Info(format, v...)
}
