package client_info

import (
	"context"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

// Runs periodically for housekeeping. This can take a long time - it
// does not matter.
func (self *Store) StartHouseKeep(
	ctx context.Context, config_obj *config_proto.Config) {

	delay := 600 * time.Second
	if config_obj.Defaults != nil {
		if config_obj.Defaults.ClientInfoHousekeepingPeriod < 0 {
			return
		}

		if config_obj.Defaults.ClientInfoHousekeepingPeriod > 0 {
			delay = time.Duration(
				config_obj.Defaults.ClientInfoHousekeepingPeriod) * time.Second
		}
	}

	go func() {
		for {
			last_run := utils.GetTime().Now()

			select {
			case <-ctx.Done():
				return

			case <-utils.GetTime().After(utils.Jitter(utils.Jitter(delay))):
				if utils.GetTime().Now().Sub(last_run) < 10*time.Second {
					utils.SleepWithCtx(ctx, time.Minute)
					continue
				}

				self.HouseKeep(ctx, config_obj)
				last_run = utils.GetTime().Now()
			}

		}
	}()
}

// This function ensures that any outstanding notifications are sent
// to clients in case any were lost when the collections were
// initially scheduled.

// We also check the client's metadata blob for any fields that need
// indexing. The fields will be synced with the data that exists in
// the client info index.
func (self *Store) HouseKeep(
	ctx context.Context, config_obj *config_proto.Config) {

	start := utils.GetTime().Now()

	defer func() {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Debug(
			"<green>ClientInfoManager</> Housekeeping in %v in %v",
			services.GetOrgName(config_obj), utils.GetTime().Now().Sub(start))
	}()

	for _, client_id := range self.Keys() {
		self.HouseKeepOutstandingTasks(ctx, config_obj, client_id)
		self.HouseKeepMetadataIndex(ctx, config_obj, client_id)
	}
}

// Check all directly connected clients and if they have any
// outstanding tasks notify them. This helps catch cases where our
// client info tasks index is out of sync for some reason so we miss
// outstanding notifications.
func (self *Store) HouseKeepOutstandingTasks(
	ctx context.Context, config_obj *config_proto.Config, client_id string) {
	notifier, err := services.GetNotifier(config_obj)
	if err != nil {
		return
	}

	if !notifier.IsClientDirectlyConnected(client_id) {
		return
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return
	}

	client_path_manager := paths.NewClientPathManager(client_id)
	tasks, err := db.ListChildren(
		config_obj, client_path_manager.TasksDirectory())
	if err != nil {
		return
	}

	if len(tasks) > 0 {
		notifier.NotifyDirectListener(client_id)
	}
}

// Periodically refresh the metadata index. Reads the indexed metadata
// fields in the client record and update the search index.
func (self *Store) HouseKeepMetadataIndex(
	ctx context.Context, config_obj *config_proto.Config, client_id string) {
	if config_obj.Defaults == nil ||
		len(config_obj.Defaults.IndexedClientMetadata) == 0 {
		return
	}

	indexed_fields := config_obj.Defaults.IndexedClientMetadata

	// Optionally update the index service.
	indexer, err := services.GetIndexer(config_obj)
	if err != nil {
		return
	}

	// Only update the record if the metadata has changed.
	_ = self.Modify(ctx, config_obj, client_id,
		func(client_info *services.ClientInfo) (
			*services.ClientInfo, error) {

			// Client id is not valid: ignore it. This can happen if
			// the client is deleted.
			if client_info == nil || client_info.Metadata == nil {
				return client_info, nil
			}

			for _, k := range indexed_fields {
				v, pres := client_info.Metadata[k]
				if pres {
					_ = indexer.SetIndex(client_id, k+":"+v)
				}
			}
			return client_info, nil
		})
}
