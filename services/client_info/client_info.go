package client_info

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/ttlcache/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/json"
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

	// Does this client have tasks outstanding?
	has_tasks TASKS_AVAILABLE_STATUS

	last_flush uint64
}

func (self *CachedInfo) SetHasTasks(status TASKS_AVAILABLE_STATUS) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.has_tasks = status
}

func (self *CachedInfo) GetHasTasks() TASKS_AVAILABLE_STATUS {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.has_tasks
}

func (self *CachedInfo) GetStats() *services.Stats {
	self.mu.Lock()
	defer self.mu.Unlock()

	return &services.Stats{
		Ping:                  self.record.Ping,
		IpAddress:             self.record.IpAddress,
		LastHuntTimestamp:     self.record.LastHuntTimestamp,
		LastEventTableVersion: self.record.LastEventTableVersion,
	}
}

func (self *CachedInfo) UpdateStats(cb func(stats *services.Stats)) {
	self.mu.Lock()

	stats := &services.Stats{}

	cb(stats)
	self.dirty = true

	if stats.Ping > 0 {
		self.record.Ping = stats.Ping
	}

	if stats.IpAddress != "" {
		self.record.IpAddress = stats.IpAddress
	}

	if stats.LastHuntTimestamp > 0 {
		self.record.LastHuntTimestamp = stats.LastHuntTimestamp
	}

	if stats.LastEventTableVersion > 0 {
		self.record.LastEventTableVersion = stats.LastEventTableVersion
	}

	// Notify other minions of the stats update.
	now := uint64(self.owner.Clock.Now().UnixNano() / 1000)
	if now > self.last_flush+1000000*MAX_PING_SYNC_SEC {
		self.mu.Unlock()

		self.owner.SendStats(self.record.ClientId, stats)

		self.Flush()
		return
	}
	self.mu.Unlock()
}

// Write ping record to data store
func (self *CachedInfo) Flush() error {
	self.mu.Lock()

	// Nothing to do
	if !self.dirty {
		self.mu.Unlock()
		return nil
	}

	ping_client_info := &actions_proto.ClientInfo{
		Ping:                  self.record.Ping,
		PingTime:              time.Unix(0, int64(self.record.Ping)*1000).String(),
		IpAddress:             self.record.IpAddress,
		LastHuntTimestamp:     self.record.LastHuntTimestamp,
		LastEventTableVersion: self.record.LastEventTableVersion,
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

	// A blind write will eventually hit the disk.
	client_path_manager := paths.NewClientPathManager(client_id)
	db.SetSubjectWithCompletion(
		self.owner.config_obj, client_path_manager.Ping(),
		ping_client_info, nil)

	return nil
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

func (self *ClientInfoManager) SendStats(client_id string, stats *services.Stats) {
	journal, err := services.GetJournal()
	if err != nil {
		return
	}

	journal.PushRowsToArtifactAsync(self.config_obj,
		ordereddict.NewDict().
			Set("ClientId", client_id).
			Set("Stats", json.MustMarshalString(stats)).
			Set("From", self.uuid),
		"Server.Internal.ClientPing")
}

func (self *ClientInfoManager) GetStats(client_id string) (*services.Stats, error) {
	cached_info, err := self.GetCacheInfo(client_id)
	if err != nil {
		return nil, err
	}

	return cached_info.GetStats(), nil
}

func (self *ClientInfoManager) UpdateStats(
	client_id string,
	cb func(stats *services.Stats)) error {
	cached_info, err := self.GetCacheInfo(client_id)
	if err != nil {
		return err
	}

	cached_info.UpdateStats(cb)
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

	// When clients are notified they need to refresh their tasks list
	// and invalidate the cache.
	err = journal.WatchQueueWithCB(ctx, config_obj, wg,
		"Server.Internal.ClientTasks", self.ProcessNotification)
	if err != nil {
		return err
	}

	// The master will be informed when new clients appear.
	is_master := services.IsMaster(config_obj)
	if is_master {
		err = journal.WatchQueueWithCB(ctx, config_obj, wg,
			"Server.Internal.ClientPing", self.ProcessPing)
		if err != nil {
			return err
		}
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
	if !pres {
		return invalidError
	}

	stats := &services.Stats{}
	serialized, pres := row.GetString("Stats")
	if !pres {
		return invalidError
	}

	err := json.Unmarshal([]byte(serialized), &stats)
	if err != nil {
		return invalidError
	}

	cached_info, err := self.GetCacheInfo(client_id)
	if err == nil {
		// Update our internal cache but do not notify further (since
		// this came from a notification anyway).
		cached_info.UpdateStats(func(cached_stats *services.Stats) {
			if stats.Ping != 0 {
				cached_stats.Ping = stats.Ping
			}

			if stats.IpAddress != "" {
				cached_stats.IpAddress = stats.IpAddress
			}

			if stats.LastHuntTimestamp > 0 {
				cached_stats.LastHuntTimestamp = stats.LastHuntTimestamp
			}

			if stats.LastEventTableVersion > 0 {
				cached_stats.LastEventTableVersion = stats.LastEventTableVersion
			}
		})
	}

	return nil
}

func (self *ClientInfoManager) Flush(client_id string) {
	cache_info, err := self.GetCacheInfo(client_id)
	if err == nil {
		cache_info.Flush()
	}
	self.lru.Remove(client_id)
}

func (self *ClientInfoManager) Clear() {
	self.lru.Purge()
}

func (self *ClientInfoManager) Get(client_id string) (*services.ClientInfo, error) {
	cached_info, err := self.GetCacheInfo(client_id)
	if err != nil {
		return nil, err
	}

	// Return a copy so it can be read safely.
	cached_info.mu.Lock()
	res := cached_info.record.Copy()
	cached_info.mu.Unlock()

	return &res, nil
}

// Load the cache info from cache or from storage.
func (self *ClientInfoManager) GetCacheInfo(client_id string) (*CachedInfo, error) {
	self.mu.Lock()
	cached_any, err := self.lru.Get(client_id)
	if err == nil {
		cache_info, ok := cached_any.(*CachedInfo)
		if ok {
			self.mu.Unlock()
			return cache_info, nil
		}
	}

	// Unlock for potentially slow filesystem operations.
	self.mu.Unlock()

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return nil, err
	}

	client_info := &services.ClientInfo{}
	client_path_manager := paths.NewClientPathManager(client_id)

	// Read the main client record
	err = db.GetSubject(self.config_obj, client_path_manager.Path(),
		&client_info.ClientInfo)
	// Special case the server - it is a special client that does not
	// need to enrol. It actually does have a client record becuase it
	// needs to schedule tasks for itself.
	if err != nil && client_id != "server" {
		return nil, err
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
		client_info.LastHuntTimestamp = ping_info.LastHuntTimestamp
		client_info.LastEventTableVersion = ping_info.LastEventTableVersion
		cache_info.last_flush = ping_info.Ping
	}

	// Set the new cached info in the lru.
	self.mu.Lock()
	self.lru.Set(client_id, cache_info)
	self.mu.Unlock()

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
	service := &ClientInfoManager{
		config_obj: config_obj,
		uuid:       utils.GetGUID(),
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
