//
package server

import (
	"context"
	"errors"
	"github.com/golang/protobuf/proto"
	"log"
	"os"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/crypto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

type Server struct {
	manager   *crypto.CryptoManager
	error_log *log.Logger
	info_log  *log.Logger
}

func NewServer(config_obj *config.Config) (*Server, error) {
	manager, err := crypto.NewServerCryptoManager(config_obj)
	if err != nil {
		return nil, err
	}
	result := Server{
		manager:   manager,
		error_log: log.New(os.Stderr, "ERR:", log.LstdFlags),
		info_log:  log.New(os.Stderr, "INFO:", log.LstdFlags),
	}
	return &result, nil
}

func (self *Server) Process(ctx context.Context, request []byte) ([]byte, error) {
	message_info, err := self.manager.Decrypt(request)
	if err != nil {
		return nil, err
	}

	message_list := &crypto_proto.MessageList{}
	err = proto.Unmarshal(message_info.Raw, message_list)
	if err != nil {
		return nil, err
	}

	for _, message := range message_list.Job {
		if message_info.Authenticated {
			auth := crypto_proto.GrrMessage_AUTHENTICATED
			message.AuthState = &auth
		}
		message.Source = message_info.Source
		processWellKnownFlow(self, message)
	}

	if !message_info.Authenticated {
		return nil, errors.New("Enrolment")
	}

	message_list = &crypto_proto.MessageList{}
	for _, response := range self.DrainRequestsForClient(*message_info.Source) {
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
	var result []*crypto_proto.GrrMessage
	return result
}

func (self *Server) Error(format string, v ...interface{}) {
	self.error_log.Printf(format, v...)
}

func (self *Server) Info(format string, v ...interface{}) {
	self.info_log.Printf(format, v...)
}
