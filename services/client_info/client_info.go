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
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	invalidError = errors.New("Invalid")
)

type ClientInfoManager struct {
	config_obj       *config_proto.Config
	uuid             int64
	mu               sync.Mutex
	mutation_manager *MutationManager

	storage *Store
}

func (self *ClientInfoManager) GetStats(
	ctx context.Context, client_id string) (*services.Stats, error) {
	record, err := self.storage.GetRecord(client_id)
	if err != nil {
		return nil, err
	}

	return &services.Stats{
		Ping:                  record.Ping,
		IpAddress:             record.IpAddress,
		LastHuntTimestamp:     record.LastHuntTimestamp,
		LastEventTableVersion: record.LastEventTableVersion,
	}, nil
}

// Checks the notification service for all currently connected clients
// so we may send the most up to date Ping information possible.
func (self *ClientInfoManager) UpdateMostRecentPing(ctx context.Context) {
	notifier, err := services.GetNotifier(self.config_obj)
	if err != nil {
		return
	}
	now := uint64(time.Now().UnixNano() / 1000)
	update_stat := &services.Stats{}
	for _, client_id := range self.mutation_manager.pings.Keys() {
		if notifier.IsClientDirectlyConnected(client_id) {
			update_stat.Ping = now
			self.UpdateStats(ctx, client_id, update_stat)
		}
	}
}

func (self *ClientInfoManager) UpdateStats(
	ctx context.Context,
	client_id string,
	stats *services.Stats) error {

	record, err := self.storage.GetRecord(client_id)
	if err != nil {
		// If a record does not exist, just make one
		record = &actions_proto.ClientInfo{
			ClientId: client_id,
		}
	}

	if stats.Ping > 0 && stats.Ping > record.Ping {
		if self.mutation_manager != nil {
			self.mutation_manager.AddPing(client_id, stats.Ping)
		}
		record.Ping = stats.Ping
	}

	if stats.IpAddress != "" &&
		stats.IpAddress != record.IpAddress {
		if self.mutation_manager != nil {
			self.mutation_manager.AddIPAddress(client_id, stats.IpAddress)
		}
		record.IpAddress = stats.IpAddress
	}

	if stats.LastHuntTimestamp > 0 &&
		stats.LastHuntTimestamp > record.LastHuntTimestamp {
		if self.mutation_manager != nil {
			self.mutation_manager.AddLastHuntTimestamp(
				client_id, stats.LastHuntTimestamp)
		}
		record.LastHuntTimestamp = stats.LastHuntTimestamp
	}

	if stats.LastEventTableVersion > 0 &&
		stats.LastEventTableVersion > record.LastEventTableVersion {
		if self.mutation_manager != nil {
			self.mutation_manager.AddLastEventTableVersion(client_id,
				stats.LastEventTableVersion)
		}
		record.LastEventTableVersion = stats.LastEventTableVersion
	}

	return self.storage.SetRecord(record)
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
	go func() {
		defer wg.Done()

		self.MutationSync(ctx, config_obj)
	}()

	// Only the master node writes to storage - there is no need to
	// flush to disk that frequently because the master keeps a hot
	// copy of the data in memory.
	if services.IsMaster(config_obj) {
		write_time := time.Duration(100) * time.Second
		if config_obj.Frontend != nil && config_obj.Frontend.Resources != nil &&
			config_obj.Frontend.Resources.ClientInfoWriteTime > 0 {
			write_time = time.Duration(
				config_obj.Frontend.Resources.ClientInfoWriteTime) * time.Second
		}

		wg.Add(1)
		go func() {
			defer wg.Done()

			// When we teardown write the data to storage if needed.
			defer self.storage.SaveSnapshot(ctx, config_obj)

			for {
				select {
				case <-ctx.Done():
					return

				case <-time.After(write_time):
					err := self.storage.SaveSnapshot(ctx, config_obj)
					if err != nil {
						logger.Error(
							"<red>ClientInfo Manager</>: writing snapshot: %v for org %v",
							err, services.GetOrgName(config_obj))
					}
				}
			}
		}()

	} else {
		// Minions watch for Server.Internal.ClientInfoSnapshot to
		// trigger their snapshot loading.
		err := journal.WatchQueueWithCB(ctx, config_obj, wg,
			"Server.Internal.ClientInfoSnapshot",
			"ClientInfoManager",
			self.ProcessSnapshotWrites)
		if err != nil {
			return err
		}
	}

	// This is a mechanism that allows clients to be notified as soon
	// as possible - without needing to wait for snapshot
	// update. Minions listen for this event and immediately update
	// the has_tasks field in the client record.
	err := journal.WatchQueueWithCB(ctx, config_obj, wg,
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

	return nil
}

func (self *ClientInfoManager) ProcessSnapshotWrites(
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

	return self.storage.LoadFromSnapshot(ctx, config_obj)
}

// Send mutations periodically
func (self *ClientInfoManager) MutationSync(
	ctx context.Context, config_obj *config_proto.Config) {

	frontend_manager, err := services.GetFrontendManager(config_obj)
	if err != nil {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Info("MutationSync: %v.", err)
		return
	}

	sync_time := time.Duration(10) * time.Second
	if config_obj.Frontend != nil && config_obj.Frontend.Resources != nil &&
		config_obj.Frontend.Resources.ClientInfoSyncTime > 0 {
		sync_time = time.Duration(config_obj.Frontend.Resources.ClientInfoSyncTime) * time.Millisecond
	}

	journal, err := services.GetJournal(config_obj)
	if err != nil {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Info("MutationSync: %v.", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return

		case <-time.After(sync_time):
			// Single server deployment does not need to sync
			// anything. We only sync on multi-frontend deployments
			// for the master to announce changes to the minions and
			// the minions to inform the master. NOTE: We still have
			// to check this at every iteration because a minion can
			// connect at any time.
			if services.IsMaster(config_obj) &&
				frontend_manager.GetMinionCount() == 0 {
				continue
			}

			// Only send a mutation if something has changed.
			size := self.mutation_manager.Size()
			if size > 0 {

				logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
				logger.Debug("ClientInfoManager: sending a mutation with %v items", size)

				// Update the ping info to the latest
				//self.UpdateMostRecentPing()

				journal.PushRowsToArtifactAsync(ctx, config_obj,
					ordereddict.NewDict().
						Set("Mutation", self.mutation_manager.GetMutation()).
						Set("From", self.uuid),
					"Server.Internal.ClientPing")
			}
		}
	}
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
			record, err := self.storage.GetRecord(client_id)
			if err == nil {
				record.Ping = uint64(value)
				self.storage.SetRecord(record)
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
			record, err := self.storage.GetRecord(client_id)
			if err == nil {
				record.IpAddress = value
				self.storage.SetRecord(record)
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

			record, err := self.storage.GetRecord(client_id)
			if err == nil {
				record.LastHuntTimestamp = uint64(value)
				self.storage.SetRecord(record)
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

			record, err := self.storage.GetRecord(client_id)
			if err == nil {
				record.LastEventTableVersion = uint64(value)
				self.storage.SetRecord(record)
			}
		}
	}

	return nil
}

func (self *ClientInfoManager) Get(
	ctx context.Context, client_id string) (*services.ClientInfo, error) {
	record, err := self.storage.GetRecord(client_id)
	if err != nil {
		return nil, err
	}

	// If the client is presently connected, then update the current
	// last_seen time because it is more accurate than the ping
	// messages written.
	notifier, err := services.GetNotifier(self.config_obj)
	if err == nil {
		if notifier.IsClientDirectlyConnected(client_id) {
			record.Ping = uint64(utils.GetTime().Now().UnixNano() / 1000)
			self.storage.SetRecord(record)
		}
	}

	return &services.ClientInfo{*record}, nil
}

func (self *ClientInfoManager) Remove(ctx context.Context, client_id string) {
	self.storage.Remove(client_id)
}

func (self *ClientInfoManager) Set(
	ctx context.Context, client_info *services.ClientInfo) error {

	if client_info.ClientId == "" {
		return invalidError
	}

	return self.storage.SetRecord(&client_info.ClientInfo)
}

func NewClientInfoManager(
	ctx context.Context,
	config_obj *config_proto.Config) (*ClientInfoManager, error) {

	// Calculate a unique id for each service.
	service := &ClientInfoManager{
		config_obj:       config_obj,
		uuid:             utils.GetGUID(),
		mutation_manager: NewMutationManager(),
	}
	service.storage = NewStorage(service.uuid)

	err := service.storage.LoadFromSnapshot(ctx, config_obj)
	if err != nil {
		return nil, err
	}

	return service, nil
}

func getDict(item *ordereddict.Dict, name string) (*ordereddict.Dict, bool) {
	res, pres := item.Get(name)
	if !pres {
		return nil, false
	}

	res_dict, ok := res.(*ordereddict.Dict)
	return res_dict, ok
}
