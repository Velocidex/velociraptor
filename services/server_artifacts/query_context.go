package server_artifacts

import (
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

// Manage each query in the same CollectionContextManager
type queryContext struct {
	id uint64

	config_obj *config_proto.Config
	session_id string
	start      time.Time

	mu     sync.Mutex
	status crypto_proto.VeloStatus
	wg     *sync.WaitGroup

	// Access the collection logger
	logger LogWriter
}

func (self *queryContext) UpdateStatus(cb func(s *crypto_proto.VeloStatus)) {
	self.mu.Lock()
	defer self.mu.Unlock()

	cb(&self.status)
	self.status.LastActive = uint64(utils.GetTime().Now().UnixNano() / 1000)
}

func (self *queryContext) GetStatus() *crypto_proto.VeloStatus {
	self.mu.Lock()
	defer self.mu.Unlock()

	return proto.Clone(&self.status).(*crypto_proto.VeloStatus)
}

func (self *queryContext) Logger() LogWriter {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.logger
}

// When the query is closed we add another status to the
// collection_context.
func (self *queryContext) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	// The query may have already been set to an error, if so leave
	// it, if not adjust it so it is completed.
	if self.status.Status == crypto_proto.VeloStatus_PROGRESS {
		self.status.Status = crypto_proto.VeloStatus_OK
	}
	self.status.Duration = (utils.GetTime().Now().UnixNano()/1000 -
		self.start.UnixNano()/1000)

	self.logger.Close()

	self.wg.Done()
}
