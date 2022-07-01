/*
   The client info manager caches client information in memory for
   quick access without having to generate IO for each client record.

   We maintain client stats as as:

   - Ping time - When the client was last seen - this is useful for the GUI

   - IpAddress - Last seen IP address

   - LastHuntTimestamp - the last hunt that was run on this
     client. This is used by the hunt dispatcher to decide which hunts
     should be assigned to the client.

   - LastEventTableVersion - the version of the client event table the
     client currently has.

   While client stats are needed on both the master and minion nodes
   our goal is to minimize IO to the filestore.

   Therefore we have the following rules:

   1. Minions can read client info from the datastore but do not
      update it - Instead they send update mutation to the
      Server.Internal.ClientPing queue.

   2. Only the master writes the update stats to storage.

   3. All other nodes (master and minions) maintain their own internal
      cache by following the Server.Internal.ClientPing queue.

   4. Update mutations are sent periodically in a combined way to
      avoid RPC overheads.
*/

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
	MAX_PING_SYNC_SEC = 10
)

// CachedInfo is an in memory cache of a single client's stats record.
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

	is_master bool
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

	// Make a copy of the cached stats
	return &services.Stats{
		Ping:                  self.record.Ping,
		IpAddress:             self.record.IpAddress,
		LastHuntTimestamp:     self.record.LastHuntTimestamp,
		LastEventTableVersion: self.record.LastEventTableVersion,
	}
}

// Update the cached stats. Cached values are only updated if needed -
// i.e. if timestamps are more advanced, or if the ip address is
// changed. If a mutation manager is provided, we also update the
// mutation.
func (self *CachedInfo) _UpdateStats(
	client_id string,
	stats *services.Stats,
	mutation_manager *MutationManager) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if stats.Ping > 0 && stats.Ping > self.record.Ping {
		self.dirty = true
		if mutation_manager != nil {
			mutation_manager.AddPing(client_id, stats.Ping)
		}
		self.record.Ping = stats.Ping
	}

	if stats.IpAddress != "" &&
		stats.IpAddress != self.record.IpAddress {
		self.dirty = true
		if mutation_manager != nil {
			mutation_manager.AddIPAddress(client_id, stats.IpAddress)
		}
		self.record.IpAddress = stats.IpAddress
	}

	if stats.LastHuntTimestamp > 0 &&
		stats.LastHuntTimestamp > self.record.LastHuntTimestamp {
		self.dirty = true
		if mutation_manager != nil {
			mutation_manager.AddLastHuntTimestamp(
				client_id, stats.LastHuntTimestamp)
		}
		self.record.LastHuntTimestamp = stats.LastHuntTimestamp
	}

	if stats.LastEventTableVersion > 0 &&
		stats.LastEventTableVersion > self.record.LastEventTableVersion {
		self.dirty = true
		if mutation_manager != nil {
			mutation_manager.AddLastEventTableVersion(client_id,
				stats.LastEventTableVersion)
		}
		self.record.LastEventTableVersion = stats.LastEventTableVersion
	}
}

// Write ping record to data store if it is dirty.
func (self *CachedInfo) Flush() error {
	self.mu.Lock()

	// Nothing to do
	if !self.dirty {
		self.mu.Unlock()
		return nil
	}

	// Only the master actually writes the client stats to storage
	if !self.is_master {
		self.dirty = false
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
		ping_client_info, utils.BackgroundWriter)

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

	is_master bool

	mutation_manager *MutationManager
}

func (self *ClientInfoManager) GetCachedClients() []string {
	return self.lru.GetKeys()
}

func (self *ClientInfoManager) GetStats(client_id string) (*services.Stats, error) {
	cached_info, err := self.GetCacheInfo(client_id)
	if err != nil {
		return nil, err
	}

	return cached_info.GetStats(), nil
}

// Checks the notification service for all currently connected clients
// so we may send the most up to date Ping information possible.
func (self *ClientInfoManager) UpdateMostRecentPing() {
	notifier, err := services.GetNotifier(self.config_obj)
	if err != nil {
		return
	}
	now := uint64(time.Now().UnixNano() / 1000)
	update_stat := &services.Stats{}
	for _, client_id := range self.mutation_manager.pings.Keys() {
		if notifier.IsClientDirectlyConnected(client_id) {
			update_stat.Ping = now
			self.UpdateStats(client_id, update_stat)
		}
	}
}

func (self *ClientInfoManager) UpdateStats(
	client_id string,
	stats *services.Stats) error {
	cached_info, err := self.GetCacheInfo(client_id)
	if err != nil {
		return err
	}

	cached_info._UpdateStats(client_id, stats, self.mutation_manager)
	return nil
}

func (self *ClientInfoManager) Start(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) error {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> Client Info service for %v.",
		services.GetOrgName(config_obj))

	// Start syncing the mutation_manager
	wg.Add(1)
	go self.MutationSync(ctx, wg, config_obj)

	// Only the master node writes to storage - there is no need to
	// flush to disk that frequently because the master keeps a hot
	// copy of the data in memory.
	if self.is_master {
		write_time := time.Duration(100) * time.Second
		if config_obj.Frontend != nil && config_obj.Frontend.Resources != nil &&
			config_obj.Frontend.Resources.ClientInfoWriteTime > 0 {
			write_time = time.Duration(
				config_obj.Frontend.Resources.ClientInfoWriteTime) *
				time.Millisecond
		}

		go func() {
			for {
				select {
				case <-ctx.Done():
					return

				case <-time.After(write_time):
					self.FlushAll()
				}
			}
		}()
	}

	// Watch for clients that are deleted and remove from local cache.
	err := journal.WatchQueueWithCB(ctx, config_obj, wg,
		"Server.Internal.ClientDelete",
		"ClientInfoManager",
		self.ProcessInterrogateResults)
	if err != nil {
		return err
	}

	// When clients are notified they need to refresh their tasks list
	// and invalidate the cache.
	err = journal.WatchQueueWithCB(ctx, config_obj, wg,
		"Server.Internal.ClientTasks",
		"ClientInfoManager",
		self.ProcessNotification)
	if err != nil {
		return err
	}

	// The master will be informed when new clients appear.
	err = journal.WatchQueueWithCB(ctx, config_obj, wg,
		"Server.Internal.ClientPing",
		"ClientInfoManager",
		self.ProcessPing)
	if err != nil {
		return err
	}

	return journal.WatchQueueWithCB(ctx, config_obj, wg,
		"Server.Internal.Interrogation",
		"ClientInfoManager",
		self.ProcessInterrogateResults)
}

func (self *ClientInfoManager) MutationSync(
	ctx context.Context, wg *sync.WaitGroup, config_obj *config_proto.Config) {
	defer wg.Done()

	sync_time := time.Duration(10) * time.Second
	if config_obj.Frontend != nil && config_obj.Frontend.Resources != nil &&
		config_obj.Frontend.Resources.ClientInfoSyncTime > 0 {
		sync_time = time.Duration(config_obj.Frontend.Resources.ClientInfoSyncTime) * time.Millisecond
	}

	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return
	}

	frontend_manager := services.GetFrontendManager()

	for {
		select {
		case <-ctx.Done():
			return

		case <-time.After(sync_time):
			// Only send a mutation if something has changed.
			size := self.mutation_manager.Size()
			if size > 0 && (!services.IsMaster(self.config_obj) ||
				frontend_manager.GetMinionCount() > 0) {

				logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
				logger.Debug("ClientInfoManager: sending a mutation with %v items", size)

				// Update the ping info to the latest
				//self.UpdateMostRecentPing()

				journal.PushRowsToArtifactAsync(config_obj,
					ordereddict.NewDict().
						Set("Mutation", self.mutation_manager.GetMutation()).
						Set("From", self.uuid),
					"Server.Internal.ClientPing")
			}
		}
	}
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

	mutation, pres := getDict(row, "Mutation")
	if !pres {
		return invalidError
	}

	ping, pres := getDict(mutation, "Ping")
	if pres {
		for _, client_id := range ping.Keys() {
			value, pres := ping.GetInt64(client_id)
			if !pres {
				continue
			}
			cached_info, err := self.GetCacheInfo(client_id)
			if err == nil {
				// Do not update the mutation manager because we do
				// not need to propagate any changes.
				cached_info._UpdateStats(client_id, &services.Stats{
					Ping: uint64(value),
				}, nil)
			}
		}
	}

	ip_addresses, pres := getDict(mutation, "IpAddress")
	if pres {
		for _, client_id := range ip_addresses.Keys() {
			value, pres := ip_addresses.GetString(client_id)
			if !pres {
				continue
			}

			cached_info, err := self.GetCacheInfo(client_id)
			if err == nil {
				// Do not update the mutation manager because we do
				// not need to propagate any changes.
				cached_info._UpdateStats(client_id, &services.Stats{
					IpAddress: value,
				}, nil)
			}
		}
	}

	last_hunt_timestamp, pres := getDict(mutation, "LastHuntTimestamp")
	if pres {
		for _, client_id := range last_hunt_timestamp.Keys() {
			value, pres := last_hunt_timestamp.GetInt64(client_id)
			if !pres {
				continue
			}

			cached_info, err := self.GetCacheInfo(client_id)
			if err == nil {
				// Do not update the mutation manager because we do
				// not need to propagate any changes.
				cached_info._UpdateStats(client_id, &services.Stats{
					LastHuntTimestamp: uint64(value),
				}, nil)
			}
		}
	}

	last_event_table_version, pres := getDict(mutation, "LastEventTableVersion")
	if pres {
		for _, client_id := range last_event_table_version.Keys() {
			value, pres := last_event_table_version.GetInt64(client_id)
			if !pres {
				continue
			}

			cached_info, err := self.GetCacheInfo(client_id)
			if err == nil {
				// Do not update the mutation manager because we do
				// not need to propagate any changes.
				cached_info._UpdateStats(client_id, &services.Stats{
					LastEventTableVersion: uint64(value),
				}, nil)
			}
		}
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

// Flush all dirty caches to disk.
func (self *ClientInfoManager) FlushAll() {
	to_flush := []*CachedInfo{}

	for _, client_id := range self.lru.GetKeys() {
		cache_info, err := self.GetCacheInfo(client_id)
		if err != nil {
			continue
		}
		cache_info.mu.Lock()
		if cache_info.dirty {
			to_flush = append(to_flush, cache_info)
		}
		cache_info.mu.Unlock()
	}

	if len(to_flush) == 0 {
		return
	}

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Debug("ClientInfoManager: Writing %v records to storage",
		len(to_flush))
	// Flush items outside the lock so we do block during IO.
	for _, item := range to_flush {
		item.Flush()
	}
}

func (self *ClientInfoManager) Clear() {
	self.lru.Purge()
}

func (self *ClientInfoManager) Remove(client_id string) {
	self.lru.Remove(client_id)
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

func (self *ClientInfoManager) Set(client_info *services.ClientInfo) error {
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	client_path_manager := paths.NewClientPathManager(client_info.ClientId)
	return db.SetSubjectWithCompletion(
		self.config_obj, client_path_manager.Path(), client_info, nil)
}

// Only look in the ttl cache - does not do any IO - best effort.
func (self *ClientInfoManager) GetCacheInfoFromCache(
	client_id string) (*CachedInfo, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	cached_any, err := self.lru.Get(client_id)
	if err != nil {
		return nil, err
	}

	cache_info, ok := cached_any.(*CachedInfo)
	if !ok {
		return nil, invalidError
	}

	return cache_info, nil
}

// Load the cache info from cache or from storage.
func (self *ClientInfoManager) GetCacheInfo(client_id string) (*CachedInfo, error) {
	cached_info, err := self.GetCacheInfoFromCache(client_id)
	if err == nil {
		return cached_info, nil
	}
	return self.GetCacheInfoFromStorage(client_id)
}

func (self *ClientInfoManager) GetCacheInfoFromStorage(
	client_id string) (*CachedInfo, error) {
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
		owner:     self,
		record:    client_info,
		is_master: self.is_master,
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
		is_master:  services.IsMaster(config_obj),

		mutation_manager: NewMutationManager(),
	}

	// When we teardown write the data to storage if needed.
	defer service.lru.Purge()

	service.lru.SetCacheSizeLimit(int(expected_clients))

	if config_obj.Frontend != nil &&
		config_obj.Frontend.Resources != nil &&
		config_obj.Frontend.Resources.ClientInfoLruTtl > 0 {
		service.lru.SetTTL(
			time.Duration(config_obj.Frontend.Resources.ClientInfoLruTtl) *
				time.Second)
	}

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

func getDict(item *ordereddict.Dict, name string) (*ordereddict.Dict, bool) {
	res, pres := item.Get(name)
	if !pres {
		return nil, false
	}

	res_dict, ok := res.(*ordereddict.Dict)
	return res_dict, ok
}
