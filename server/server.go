/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2022 Rapid7 Inc.

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
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/juju/ratelimit"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/crypto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	crypto_server "www.velocidex.com/golang/velociraptor/crypto/server"
	"www.velocidex.com/golang/velociraptor/flows"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	targetConcurrency = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "frontend_maximum_concurrency",
		Help: "Maximum number of clients we serve at the same time.",
	})

	heapSize = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "frontend_current_heap_size",
		Help: "Size of allocated heap.",
	})
)

type Server struct {
	manager *crypto_server.ServerCryptoManager
	logger  *logging.LogContext

	// Limit concurrency for processing messages.
	mu                  sync.Mutex
	concurrency         *utils.Concurrency
	concurrency_timeout time.Duration
	throttler           *utils.Throttler

	// The server dynamically adjusts concurrency. This signals exit.
	done chan bool

	Bucket  *ratelimit.Bucket
	Healthy int32
}

func (self *Server) Concurrency() *utils.Concurrency {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.concurrency
}

func (self *Server) adjustConcurrency(
	max_concurrency uint64,
	target_heap_size uint64, concurrency uint64) uint64 {

	s := runtime.MemStats{}
	runtime.ReadMemStats(&s)

	// We are using up too much memory, drop concurrency.
	if s.Alloc > target_heap_size {
		new_concurrency := 2 * concurrency / 3

		// We need some minimum concurrency (default 10% of max)
		if new_concurrency < max_concurrency/10 {
			new_concurrency = max_concurrency / 10
		}

		// No change in concurrency - nothing to do.
		if new_concurrency == concurrency {
			return concurrency
		}

		// Adjust concurrency
		self.logger.Debug("Adjusting concurrency from %v to %v", concurrency, new_concurrency)
		concurrency = new_concurrency

		// We are using up less memory than
		// specified, we can increase
		// concurrency to approach the maximum level.
	} else if s.Alloc < 2*target_heap_size/3 {
		delta := (max_concurrency - concurrency) / 2

		// No change in concurrency - nothing to do.
		if delta == 0 {
			return concurrency
		}

		// Adjust concurrency
		self.logger.Debug("Adjusting concurrency from %v to %v", concurrency, concurrency+delta)
		concurrency += delta

	} else {
		// Heap size is between max and 2/3 of max - this is
		// good so no change is needed, check again later.
		return concurrency
	}

	// Install the new concurrency controller.
	targetConcurrency.Set(float64(concurrency))
	heapSize.Set(float64(s.Alloc))
	self.mu.Lock()
	self.concurrency = utils.NewConcurrencyControl(int(concurrency), self.concurrency_timeout)
	self.mu.Unlock()

	return concurrency
}

func (self *Server) ManageConcurrency(max_concurrency uint64, target_heap_size uint64) {
	concurrency := max_concurrency / 2

	for {
		new_concurrency := self.adjustConcurrency(max_concurrency, target_heap_size, concurrency)
		concurrency = new_concurrency

		// Wait for a minute and check again.
		select {
		case <-self.done:
			return

		case <-time.After(time.Minute):
		}
	}
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
		concurrency_timeout: time.Duration(concurrency) * time.Second,
		done:                make(chan bool),
	}

	result.concurrency = utils.NewConcurrencyControl(
		int(concurrency), result.concurrency_timeout)

	if config_obj.Frontend.Resources.ConnectionsPerSecond > 0 {
		result.logger.Info("Throttling connections to %v QPS",
			config_obj.Frontend.Resources.ConnectionsPerSecond)
		result.throttler = utils.NewThrottler(config_obj.Frontend.Resources.ConnectionsPerSecond)
	}

	heap_size := config_obj.Frontend.Resources.TargetHeapSize
	if heap_size > 0 {
		// If we are targetting a heap size then regulate concurrency
		result.logger.Info("Targetting heap size %v, with maximum concurrency %v",
			heap_size, concurrency)

		go result.ManageConcurrency(concurrency, heap_size)
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
	message *crypto_proto.VeloMessage) {

	if message.CSR != nil {
		org_manager, err := services.GetOrgManager()
		if err != nil {
			return
		}

		config_obj, err := org_manager.GetOrgConfig(message.OrgId)
		if err != nil {
			return
		}

		err = enroll(ctx, config_obj, self, message.CSR)
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

	// json.TraceMessage(message_info.Source, message_info)

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return nil, 0, err
	}

	config_obj, err := org_manager.GetOrgConfig(message_info.OrgId)
	if err != nil {
		return nil, 0, err
	}

	runner := flows.NewFlowRunner(config_obj)
	defer runner.Close()

	err = runner.ProcessMessages(ctx, message_info)
	if err != nil {
		return nil, 0, err
	}

	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return nil, 0, err
	}
	err = client_info_manager.UpdateStats(message_info.Source,
		&services.Stats{
			Ping:      uint64(time.Now().UnixNano() / 1000),
			IpAddress: message_info.RemoteAddr,
		})
	if err != nil {
		return nil, 0, err
	}

	message_list := &crypto_proto.MessageList{}
	if drain_requests_for_client {
		tasks, err := client_info_manager.GetClientTasks(message_info.Source)
		if err == nil {
			message_list.Job = append(message_list.Job, tasks...)
		}
	}

	/*
		for i := 0; i < len(message_list.Job); i++ {
			json.TraceMessage(message_info.Source+"_out", message_list.Job[i])
		}
	*/

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

func (self *Server) Error(format string, v ...interface{}) {
	self.logger.Error(format, v...)
}

func (self *Server) Info(format string, v ...interface{}) {
	self.logger.Info(format, v...)
}

func (self *Server) Debug(format string, v ...interface{}) {
	self.logger.Debug(format, v...)
}
