package hunt_dispatcher

import (
	"context"
	"sort"
	"strings"
	"sync/atomic"
	"time"

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

	start := utils.GetTime().Now()
	if start.Sub(self.last_flush_time) < 5*time.Second {
		return nil
	}

	return self._FlushIndex(ctx)
}

func (self *HuntStorageManagerImpl) _FlushIndex(
	ctx context.Context) error {

	// Nothing to do because none of the records are dirty.
	if !self.dirty {
		return nil
	}

	if atomic.LoadInt64(&self.closed) > 0 {
		return nil
	}

	// Debounce the flushing a bit so we dont overload the system for
	// fast events. Note that flushes occur periodically anyway so if
	// we skip a flush we will get it later.
	start := utils.GetTime().Now()
	self.last_flush_time = start

	hunt_ids := make([]string, 0, len(self.hunts))
	for hunt_id := range self.hunts {
		hunt_ids = append(hunt_ids, hunt_id)
	}

	defer func() {
		now := utils.GetTime().Now()
		self.last_flush_time = now

		logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
		logger.Debug("HuntDispatcher: <green>Rebuilt Hunt Index in %v for %v (%v hunts)</>",
			now.Sub(start), services.GetOrgName(self.config_obj), len(hunt_ids))
	}()

	// Result set does not exist - recreate it
	hunt_path_manager := paths.NewHuntPathManager("")
	file_store_factory := file_store.GetFileStore(self.config_obj)
	rs_writer, err := result_sets.NewResultSetWriter(file_store_factory,
		hunt_path_manager.HuntIndex(), json.DefaultEncOpts(),

		// We need the index to be written immediately so it is
		// visible in the GUI.
		utils.SyncCompleter,
		result_sets.TruncateMode)
	if err != nil {
		return err
	}
	defer rs_writer.Close()

	// Sort hunts by hunt id so latest hunt is at the top of the
	// table.
	sort.Sort(sort.Reverse(sort.StringSlice(hunt_ids)))

	for _, hunt_id := range hunt_ids {
		hunt_record, pres := self.hunts[hunt_id]
		if !pres {
			continue
		}

		// Skip archived hunts
		if hunt_record.State == api_proto.Hunt_ARCHIVED {
			continue
		}

		// Clean all the records. By the time this function exits all
		// records should be clean.
		if hunt_record.dirty {
			hunt_record.dirty = false
			serialized, err := json.Marshal(hunt_record.Hunt)
			if err == nil {
				hunt_record.serialized = serialized
			}
			self.hunts[hunt_id] = hunt_record
		}

		rs_writer.Write(ordereddict.NewDict().
			Set("HuntId", hunt_record.HuntId).
			Set("Description", hunt_record.HuntDescription).
			// Store the tags in the index so we can search for them.
			Set("Tags", strings.Join(hunt_record.Tags, "\n")).
			Set("Created", hunt_record.CreateTime).
			Set("Started", hunt_record.StartTime).
			Set("Expires", hunt_record.Expires).
			Set("Creator", hunt_record.Creator).
			Set("Hunt", hunt_record.serialized))
	}

	self.dirty = false

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
