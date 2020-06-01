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
	"fmt"
	"time"

	"github.com/juju/ratelimit"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/crypto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/flows"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
)

var (
	concurrencyControl = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "client_comms_concurrency",
		Help: "The total number of currently executing client recerive operations",
	})
)

type Server struct {
	config  *config_proto.Config
	manager *crypto.CryptoManager
	logger  *logging.LogContext
	db      datastore.DataStore

	// Limit concurrency for processing messages.
	concurrency chan bool

	Bucket           *ratelimit.Bucket
	APIClientFactory grpc_client.APIClientFactory
}

func (self *Server) StartConcurrencyControl() {
	// Wait here until we have enough room in the concurrency
	// channel.
	self.concurrency <- true
	concurrencyControl.Inc()
}

func (self *Server) EndConcurrencyControl() {
	<-self.concurrency
	concurrencyControl.Dec()
}

func (self *Server) Close() {
	self.db.Close()
}

func NewServer(
	config_obj *config_proto.Config) (*Server, error) {
	manager, err := crypto.NewServerCryptoManager(config_obj)
	if err != nil {
		return nil, err
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	// This number mainly affects memory use during large tranfers
	// as it controls the number of concurrent clients that may be
	// tranferring data (each will use some memory to buffer).
	concurrency := config_obj.Frontend.Concurrency
	if concurrency == 0 {
		concurrency = 6
	}

	result := Server{
		config:  config_obj,
		manager: manager,
		db:      db,
		logger: logging.GetLogger(config_obj,
			&logging.FrontendComponent),
		concurrency:      make(chan bool, concurrency),
		APIClientFactory: grpc_client.GRPCAPIClient{},
	}

	if config_obj.Frontend.GlobalUploadRate > 0 {
		result.logger.Info("Global upload rate set to %v bytes per second",
			config_obj.Frontend.GlobalUploadRate)
		result.Bucket = ratelimit.NewBucketWithRate(
			float64(config_obj.Frontend.GlobalUploadRate),
			1024*1024)
	}

	return &result, nil
}

// We only process enrollment messages when the client is not fully
// authenticated.
func (self *Server) ProcessSingleUnauthenticatedMessage(
	ctx context.Context,
	message *crypto_proto.GrrMessage) {
	if message.CSR != nil {
		err := enroll(ctx, self, message.CSR)
		if err != nil {
			self.logger.Error(fmt.Sprintf("Enrol Error: %s", err))
		}
	}
}

func (self *Server) ProcessUnauthenticatedMessages(
	ctx context.Context,
	message_info *crypto.MessageInfo) error {

	return message_info.IterateJobs(ctx, self.ProcessSingleUnauthenticatedMessage)
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

	runner := flows.NewFlowRunner(self.config)
	defer runner.Close()

	err := runner.ProcessMessages(ctx, message_info)
	if err != nil {
		return nil, 0, err
	}

	// Record some stats about the client.
	client_info := &actions_proto.ClientInfo{
		Ping:      uint64(time.Now().UTC().UnixNano() / 1000),
		IpAddress: message_info.RemoteAddr,
	}

	client_path_manager := paths.NewClientPathManager(message_info.Source)
	err = self.db.SetSubject(
		self.config, client_path_manager.Ping().Path(), client_info)
	if err != nil {
		return nil, 0, err
	}

	message_list := &crypto_proto.MessageList{}
	if drain_requests_for_client {
		message_list.Job = append(
			message_list.Job,
			self.DrainRequestsForClient(message_info.Source)...)
	}

	// Messages sent to clients are typically small and we do not
	// benefit from compression.
	response, err := self.manager.EncryptMessageList(
		message_list,
		crypto_proto.PackedMessageList_UNCOMPRESSED,
		message_info.Source)
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
