package hunt_dispatcher

import (
	"context"
	"sort"

	"github.com/Velocidex/ordereddict"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self *HuntStorageManagerImpl) FlushIndex(
	ctx context.Context) error {
	// Only the master flushes the records
	if !self.I_am_master {
		return nil
	}

	self.mu.Lock()
	defer self.mu.Unlock()

	hunt_ids := make([]string, 0, len(self.hunts))
	for hunt_id := range self.hunts {
		hunt_ids = append(hunt_ids, hunt_id)
	}

	start := utils.GetTime().Now()
	defer func() {
		now := utils.GetTime().Now()

		logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
		logger.Info("HuntDispatcher: <green>Rebuilt Hunt Index in %v for %v (%v hunts)</>",
			now.Sub(start), services.GetOrgName(self.config_obj), len(hunt_ids))
	}()

	// Result set does not exist - recreate it
	hunt_path_manager := paths.NewHuntPathManager("")
	file_store_factory := file_store.GetFileStore(self.config_obj)
	rs_writer, err := result_sets.NewResultSetWriter(file_store_factory,
		hunt_path_manager.HuntIndex(),
		json.DefaultEncOpts(), utils.SyncCompleter, result_sets.TruncateMode)
	if err != nil {
		return err
	}
	defer rs_writer.Close()

	// Sort hunts by hunt id so latest hunt is at the top of the
	// table.
	sort.Strings(hunt_ids)

	for _, hunt_id := range hunt_ids {
		hunt_record, pres := self.hunts[hunt_id]
		if !pres {
			continue
		}

		// Skip archived hunts
		if hunt_record.State == api_proto.Hunt_ARCHIVED {
			continue
		}

		if hunt_record.dirty {
			serialized, err := json.Marshal(hunt_record.Hunt)
			if err == nil {
				hunt_record.serialized = serialized
			}
		}

		rs_writer.Write(ordereddict.NewDict().
			Set("HuntId", hunt_record.HuntId).
			Set("Description", hunt_record.HuntDescription).
			Set("Created", hunt_record.CreateTime).
			Set("Started", hunt_record.StartTime).
			Set("Expires", hunt_record.Expires).
			Set("Creator", hunt_record.Creator).
			Set("Hunt", hunt_record.serialized))
	}
	return nil
}

// Gets the hunts by pages
func (self *HuntDispatcher) GetHunts(ctx context.Context,
	config_obj *config_proto.Config,
	options result_sets.ResultSetOptions,
	start_row, length int64) ([]*api_proto.Hunt, int64, error) {

	hunts, total, err := self.Store.ListHunts(
		ctx, options, start_row, length)
	if err != nil {
		return nil, 0, err
	}

	// Enrich the stored hunt index with live data from the hunt
	// dispatcher.
	result := make([]*api_proto.Hunt, 0, len(hunts))
	for _, hunt := range hunts {
		full_obj, ok := self.GetHunt(ctx, hunt.HuntId)
		if ok {
			result = append(result, full_obj)
		}
	}

	return result, total, nil
}
