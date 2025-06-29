package client_info

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
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
		last_run := utils.GetTime().Now()

		for {
			select {
			case <-ctx.Done():
				return

			case <-utils.GetTime().After(utils.Jitter(utils.Jitter(delay))):
				if utils.GetTime().Now().Sub(last_run) < 10*time.Second {
					utils.SleepWithCtx(ctx, time.Minute)
					continue
				}

				self.houseKeep(ctx, config_obj)
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
func (self *Store) houseKeep(
	ctx context.Context, config_obj *config_proto.Config) {

	start := utils.GetTime().Now()

	defer func() {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Debug(
			"<green>ClientInfoManager</> Housekeeping in %v in %v",
			services.GetOrgName(config_obj), utils.GetTime().Now().Sub(start))
	}()

	for _, client_id := range self.Keys() {
		self.houseKeepOutstandingTasks(ctx, config_obj, client_id)
		self.houseKeepMetadataIndex(ctx, config_obj, client_id)
	}
}

// Check all directly connected clients and if they have any
// outstanding tasks notify them. This helps catch cases where our
// client info tasks index is out of sync for some reason so we miss
// outstanding notifications.
func (self *Store) houseKeepOutstandingTasks(
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

func (self *Store) houseKeepMetadataIndex(
	ctx context.Context, config_obj *config_proto.Config, client_id string) {

	metadata, err := self.GetMetadata(ctx, config_obj, client_id)
	if err != nil {
		return
	}

	_ = self.updateClientMetadataIndex(ctx, config_obj, client_id, metadata)
}

func (self *Store) updateClientMetadataIndex(
	ctx context.Context, config_obj *config_proto.Config,
	client_id string, metadata *ordereddict.Dict) error {

	if config_obj.Defaults == nil ||
		len(config_obj.Defaults.IndexedClientMetadata) == 0 {
		return nil
	}

	indexed_fields := config_obj.Defaults.IndexedClientMetadata

	// Optionally update the index service.
	indexer, err := services.GetIndexer(config_obj)
	if err != nil {
		return err
	}

	// Only update the record if the metadata has changed.
	return self.Modify(ctx, config_obj, client_id,
		func(client_info *services.ClientInfo) (
			*services.ClientInfo, error) {

			// Client id is not valid: ignore it. This can happen if
			// the client is deleted.
			if client_info == nil {
				return client_info, nil
			}

			changed := false

			if client_info.Metadata == nil {
				client_info.Metadata = make(map[string]string)
			}

			for _, k := range indexed_fields {
				new_v, new_v_pres := metadata.GetString(k)
				old_v, old_v_pres := client_info.Metadata[k]
				if new_v_pres && !old_v_pres {
					client_info.Metadata[k] = new_v
				} else if old_v_pres && !new_v_pres {
					delete(client_info.Metadata, k)
				} else if new_v != old_v {
					client_info.Metadata[k] = new_v
				} else {
					// Value has not changed, skip updates
					continue
				}

				// Also update the search index with the new data so it is
				// immediately visible.
				if indexer != nil {
					_ = indexer.UnsetIndex(client_id, k+":"+old_v)
					_ = indexer.SetIndex(client_id, k+":"+new_v)
				}
				changed = true
			}

			if !changed {
				return nil, nil
			}

			return client_info, nil
		})
}
