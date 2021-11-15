package client_info

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ReneKroon/ttlcache/v2"
	"github.com/Velocidex/ordereddict"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	metricLRUCount = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "client_info_lru_size",
			Help: "Number of entries in client lru",
		})

	invalidError = errors.New("Invalid")
)

const (
	MAX_CACHE_AGE     = 3600
	MAX_PING_SYNC_SEC = 10
)

type CachedInfo struct {
	mu sync.Mutex

	owner *ClientInfoManager

	// Fresh data may not be flushed yet.
	dirty bool

	// Data on disk
	record *services.ClientInfo

	last_flush uint64
}

// last seen is in uS
func (self *CachedInfo) UpdatePing(
	last_seen uint64, ip_address string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if last_seen == 0 {
		self.record.Ping = uint64(self.owner.Clock.Now().UnixNano() / 1000)
	} else {
		self.record.Ping = last_seen
	}

	if ip_address != "" {
		self.record.IpAddress = ip_address
	}
	self.dirty = true
}

// Update the ping and notify other client info managers of the change.
func (self *CachedInfo) UpdatePingAndNotify(
	last_seen uint64, ip_address string) {
	self.mu.Lock()

	if last_seen == 0 {
		self.record.Ping = uint64(self.owner.Clock.Now().UnixNano() / 1000)
	} else {
		self.record.Ping = last_seen
	}

	if ip_address != "" {
		self.record.IpAddress = ip_address
	}
	self.dirty = true

	// Notify of the ping.
	if self.record.Ping > self.last_flush+1000000*MAX_PING_SYNC_SEC {
		self.mu.Unlock()

		self.owner.SendPing(
			self.record.ClientId, ip_address, self.record.Ping)
		return

		self.Flush()
		return
	}

	self.mu.Unlock()
}

// Write ping record to data store
func (self *CachedInfo) Flush() error {
	// Nothing to do
	if !self.dirty {
		return nil
	}

	self.mu.Lock()
	ping_client_info := &actions_proto.ClientInfo{
		Ping:      self.record.Ping,
		IpAddress: self.record.IpAddress,
	}
	client_id := self.record.ClientId
	self.dirty = false
	self.last_flush = uint64(self.owner.Clock.Now().UnixNano() / 1000)
	self.mu.Unlock()

	// Record some stats about the client.
	db, err := datastore.GetDB(self.owner.config_obj)
	if err != nil {
		return err
	}

	client_path_manager := paths.NewClientPathManager(client_id)
	return db.SetSubject(
		self.owner.config_obj, client_path_manager.Ping(), ping_client_info)
}

type ClientInfoManager struct {
	config_obj *config_proto.Config

	// Stores client_id -> *CachedInfo
	lru *ttlcache.Cache

	Clock utils.Clock

	uuid int64

	mu    sync.Mutex
	queue []*ordereddict.Dict
}

func (self *ClientInfoManager) SendPing(client_id, ip_address string, ping uint64) {
	self.mu.Lock()
	self.queue = append(self.queue, ordereddict.NewDict().
		Set("ClientId", client_id).
		Set("Ping", ping).
		Set("IpAddress", ip_address).
		Set("From", self.uuid))
	self.mu.Unlock()
}

func (self *ClientInfoManager) UpdatePing(client_id, ip string) error {
	cached_info, err := self.getCacheInfo(client_id)
	if err != nil {
		return err
	}

	cached_info.UpdatePingAndNotify(0, ip)
	return nil
}

func (self *ClientInfoManager) Start(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) error {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> Client Info service.")

	// Watch for clients that are deleted and remove from local cache.
	err := journal.WatchQueueWithCB(ctx, config_obj, wg,
		"Server.Internal.ClientDelete", self.ProcessInterrogateResults)
	if err != nil {
		return err
	}

	// The master will be informed when new clients appear.
	if services.GetFrontendManager().IsMaster() {
		err = journal.WatchQueueWithCB(ctx, config_obj, wg,
			"Server.Internal.ClientPing", self.ProcessPing)
		if err != nil {
			return err
		}
	} else {
		journal_manager, err := services.GetJournal()
		if err != nil {
			return err
		}

		// Minions will push rows to the master to inform it about the
		// new ping times.
		wg.Add(1)
		go func() {
			defer wg.Done()

			for {
				select {
				case <-ctx.Done():
					return

				case <-time.After(10 * time.Second):
					self.mu.Lock()
					queue := self.queue
					self.queue = nil
					self.mu.Unlock()

					// Push the rows to the master.
					if len(queue) > 0 {
						err = journal_manager.PushRowsToArtifact(
							self.config_obj, queue,
							"Server.Internal.ClientPing", "server", "")
						if err != nil {
							fmt.Printf("RPC Error: %v\n", err)
						}
					}
				}
			}
		}()
	}
	return journal.WatchQueueWithCB(ctx, config_obj, wg,
		"Server.Internal.Interrogation", self.ProcessInterrogateResults)
}

func (self *ClientInfoManager) ProcessInterrogateResults(
	ctx context.Context, config_obj *config_proto.Config,
	row *ordereddict.Dict) error {
	client_id, pres := row.GetString("ClientId")
	if pres {
		self.lru.Remove(client_id)
	}
	return nil
}

func (self *ClientInfoManager) ProcessPing(
	ctx context.Context, config_obj *config_proto.Config,
	row *ordereddict.Dict) error {

	from, pres := row.GetInt64("From")
	if !pres || from == 0 {
		return invalidError
	}

	// Ignore messages coming from us.
	if from == self.uuid {
		return nil
	}

	client_id, pres := row.GetString("ClientId")
	if !pres || client_id == "" {
		return invalidError
	}

	ping, pres := row.GetInt64("Ping")
	if !pres || ping == 0 {
		return invalidError
	}

	ip_address, pres := row.GetString("IpAddress")
	if !pres {
		return invalidError
	}

	cached_info, err := self.getCacheInfo(client_id)
	if err == nil {
		// Update our internal cache but do not notify further (since
		// this came from a notification anyway).
		cached_info.UpdatePing(uint64(ping), ip_address)
	}

	return nil
}

func (self *ClientInfoManager) Flush(client_id string) {
	cache_info, err := self.getCacheInfo(client_id)
	if err == nil {
		cache_info.Flush()
	}
	self.lru.Remove(client_id)
}

func (self *ClientInfoManager) Clear() {
	self.lru.Purge()
}

func (self *ClientInfoManager) Get(client_id string) (*services.ClientInfo, error) {
	cached_info, err := self.getCacheInfo(client_id)
	if err != nil {
		return nil, err
	}

	return cached_info.record, nil
}

func (self *ClientInfoManager) getCacheInfo(client_id string) (*CachedInfo, error) {
	cached_any, err := self.lru.Get(client_id)
	if err == nil {
		cache_info, ok := cached_any.(*CachedInfo)
		if ok {
			return cache_info, nil
		}
	}

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return nil, err
	}

	client_info := &services.ClientInfo{}
	client_path_manager := paths.NewClientPathManager(client_id)

	// Read the main client record
	err = db.GetSubject(self.config_obj, client_path_manager.Path(),
		&client_info.ClientInfo)
	if err != nil {
		// Client record does not exist on disk - Make a fresh one
		// because the client may not have been interrogated yet.
		client_info.ClientId = client_id
	}

	cache_info := &CachedInfo{
		owner:  self,
		record: client_info,
	}

	// Now read the ping info in case it is there.
	ping_info := &services.ClientInfo{}
	err = db.GetSubject(self.config_obj, client_path_manager.Ping(), ping_info)
	if err == nil {
		client_info.Ping = ping_info.Ping
		client_info.IpAddress = ping_info.IpAddress
		cache_info.last_flush = ping_info.Ping
	}

	self.lru.Set(client_id, cache_info)
	metricLRUCount.Inc()

	return cache_info, nil
}

func NewClientInfoManager(config_obj *config_proto.Config) *ClientInfoManager {
	expected_clients := int64(100)
	if config_obj.Frontend != nil &&
		config_obj.Frontend.Resources != nil &&
		config_obj.Frontend.Resources.ExpectedClients > 0 {
		expected_clients = config_obj.Frontend.Resources.ExpectedClients
	}

	// Calculate a unique id for each service.
	u := uuid.New()
	service := &ClientInfoManager{
		config_obj: config_obj,
		uuid:       int64(binary.BigEndian.Uint64(u[0:8])),
		lru:        ttlcache.NewCache(),
		Clock:      &utils.RealClock{},
	}

	// When we teardown write the data to storage if needed.
	defer service.lru.Purge()

	service.lru.SetCacheSizeLimit(int(expected_clients))
	service.lru.SetTTL(MAX_CACHE_AGE * time.Second)

	// Keep track of the total number of items in the lru.
	service.lru.SetNewItemCallback(func(key string, value interface{}) {
		metricLRUCount.Inc()
	})

	service.lru.SetExpirationCallback(func(key string, value interface{}) {
		info, ok := value.(*CachedInfo)
		if ok {
			info.Flush()
		}
		metricLRUCount.Dec()
	})

	return service
}

func StartClientInfoService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	service := NewClientInfoManager(config_obj)
	services.RegisterClientInfoManager(service)

	return service.Start(ctx, config_obj, wg)
}
