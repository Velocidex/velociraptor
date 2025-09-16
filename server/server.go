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
//
package server

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"time"

	"github.com/juju/ratelimit"
	"github.com/prometheus/client_golang/prometheus"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/crypto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	crypto_server "www.velocidex.com/golang/velociraptor/crypto/server"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/flows"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

const (
	DrainRequestsForClient      = true
	DoNotDrainRequestsForClient = false
)

type Server struct {
	Healthy int32

	manager *crypto_server.ServerCryptoManager
	logger  *logging.LogContext

	// Limit concurrency for processing messages.
	mu                  sync.Mutex
	concurrency         *utils.Concurrency
	reader_concurrency  *utils.Concurrency
	concurrency_timeout time.Duration
	throttler           *utils.Throttler

	// The server dynamically adjusts concurrency. This signals exit.
	done chan bool

	Bucket *ratelimit.Bucket
}

func (self *Server) Concurrency() *utils.Concurrency {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.concurrency
}

func (self *Server) ReaderConcurrency() *utils.Concurrency {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.reader_concurrency
}

func (self *Server) Close() {
	close(self.done)
	if self.throttler != nil {
		self.throttler.Close()
	}
}

func NewServer(ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) (*Server, error) {
	if config_obj.Frontend == nil {
		return nil, errors.New("Frontend not configured")
	}

	manager, err := crypto_server.NewServerCryptoManager(ctx, config_obj, wg)
	if err != nil {
		return nil, err
	}

	// This number mainly affects memory use during large tranfers
	// as it controls the number of concurrent clients that may be
	// transferring data (each will use some memory to
	// buffer). This should not be too large relative to the
	// available CPU cores.
	concurrency := config_obj.Frontend.Resources.Concurrency
	if concurrency == 0 {
		concurrency = 2 * uint64(runtime.GOMAXPROCS(0))
	}

	concurrency_timeout := config_obj.Frontend.Resources.ConcurrencyTimeout
	if concurrency_timeout == 0 {
		concurrency_timeout = 600
	}

	result := Server{
		manager:             manager,
		logger:              logging.GetLogger(config_obj, &logging.FrontendComponent),
		concurrency_timeout: time.Duration(concurrency_timeout) * time.Second,
		done:                make(chan bool),
	}

	result.concurrency = utils.NewConcurrencyControl(
		int(concurrency), result.concurrency_timeout)

	result.reader_concurrency = utils.NewConcurrencyControl(
		int(100), result.concurrency_timeout)

	if config_obj.Frontend.Resources.ConnectionsPerSecond > 0 {
		result.logger.Info("Throttling connections to %v QPS",
			config_obj.Frontend.Resources.ConnectionsPerSecond)
		result.throttler = utils.NewThrottler(config_obj.Frontend.Resources.ConnectionsPerSecond)
	}

	if config_obj.Frontend.Resources.GlobalUploadRate > 0 {
		result.logger.Info("Global upload rate set to %v bytes per second",
			config_obj.Frontend.Resources.GlobalUploadRate)
		result.Bucket = ratelimit.NewBucketWithRate(
			float64(config_obj.Frontend.Resources.GlobalUploadRate),
			1024*1024)
	}

	return &result, nil
}

// We only process enrollment messages when the client is not fully
// authenticated.
func (self *Server) ProcessSingleUnauthenticatedMessage(
	ctx context.Context,
	message *crypto_proto.VeloMessage) error {

	if message.CSR != nil {
		org_manager, err := services.GetOrgManager()
		if err != nil {
			return err
		}

		config_obj, err := org_manager.GetOrgConfig(message.OrgId)
		if err != nil {
			return err
		}

		err = enroll(ctx, config_obj, self, message.CSR)
		if err != nil {
			self.logger.Error("Enrol Error: %s", err)
		}
		return err
	}

	return nil
}

func (self *Server) ProcessUnauthenticatedMessages(
	ctx context.Context, config_obj *config_proto.Config,
	message_info *crypto.MessageInfo) error {

	return message_info.IterateJobs(ctx, config_obj,
		self.ProcessSingleUnauthenticatedMessage)
}

func (self *Server) DecryptForReader(ctx context.Context, request []byte) (
	*crypto.MessageInfo, error) {

	// Keep track of the average time the request spends
	// waiting for a concurrency slot. If this time is too
	// long it means concurrency may need to be increased.
	timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
		concurrencyWaitHistorgram.Observe(v)
	}))

	cancel, err := self.ReaderConcurrency().StartConcurrencyControl(ctx)
	if err != nil {
		timeoutCounter.Inc()
		return nil, errors.New("Timeout")
	}
	defer cancel()
	timer.ObserveDuration()

	return self.Decrypt(ctx, request)
}

func (self *Server) Decrypt(ctx context.Context, request []byte) (
	*crypto.MessageInfo, error) {

	message_info, err := self.manager.Decrypt(ctx, request)
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

	// json.TraceMessage(message_info.Source, message_info)

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return nil, 0, err
	}

	config_obj, err := org_manager.GetOrgConfig(message_info.OrgId)
	if err != nil {
		return nil, 0, err
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, 0, err
	}

	// If the datastore is not healthy refuse to accept this
	// connection.
	err = db.Healthy()
	if err != nil {
		return nil, 0, err
	}

	// Older clients
	if message_info.Version < constants.CLIENT_API_VERSION_0_6_8 {
		runner := flows.NewLegacyFlowRunner(config_obj)
		defer runner.Close(ctx)

		err = runner.ProcessMessages(ctx, message_info)
	} else {

		// Newer clients maintain flow state on the client so need a
		// much cheaper flow runner.
		runner := flows.NewFlowRunner(ctx, config_obj)
		defer runner.Close(ctx)

		err = runner.ProcessMessages(ctx, message_info)
	}

	if err != nil {
		return nil, 0, err
	}

	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return nil, 0, err
	}
	err = client_info_manager.UpdateStats(ctx, message_info.Source,
		&services.Stats{
			Ping:      uint64(utils.Now().UnixNano() / 1000),
			IpAddress: message_info.RemoteAddr,
		})
	if err != nil {

		// If we can not read the client_info from disk, we cache a
		// fake client_info. This can happen during enrollment when a
		// proper client_info is not written yet but hunts are still
		// outstanding. Eventually the real client info will be
		// properly updated.
		err = client_info_manager.Set(ctx, &services.ClientInfo{
			ClientInfo: &actions_proto.ClientInfo{
				ClientId:  message_info.Source,
				Ping:      uint64(utils.Now().UnixNano() / 1000),
				IpAddress: message_info.RemoteAddr,
			}})
		if err != nil {
			return nil, 0, err
		}
	}

	message_list := &crypto_proto.MessageList{}

	// Check if any messages are queued for the client. This also
	// checks for any outstanding status checks.
	if drain_requests_for_client {
		tasks, err := client_info_manager.GetClientTasks(ctx, message_info.Source)
		if err == nil {
			message_list.Job = append(message_list.Job, tasks...)
		}
	}

	/*
		for i := 0; i < len(message_list.Job); i++ {
			json.TraceMessage(message_info.Source+"_out", message_list.Job[i])
		}
	*/

	nonce := ""
	if config_obj.Client != nil {
		nonce = config_obj.Client.Nonce
	}

	// Send the client any outstanding tasks.
	response, err := self.manager.EncryptMessageList(
		message_list,

		// Messages sent to clients are typically small and do not
		// benefit from compression.
		crypto_proto.PackedMessageList_UNCOMPRESSED,
		nonce,
		message_info.Source)
	if err != nil {
		return nil, 0, err
	}

	return response, len(message_list.Job), nil
}

func (self *Server) Error(format string, v ...interface{}) {
	self.logger.Error(format, v...)
}

func (self *Server) Info(format string, v ...interface{}) {
	self.logger.Info(format, v...)
}

func (self *Server) Debug(format string, v ...interface{}) {
	self.logger.Debug(format, v...)
}
