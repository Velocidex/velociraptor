package hunt_dispatcher

import (
	"context"
	"encoding/base64"
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

func (self *HuntStorageManagerImpl) FlushIndex(ctx context.Context) (int, error) {
	// Only the master flushes the records
	if !self.I_am_master {
		return 0, nil
	}

	self.mu.Lock()
	defer self.mu.Unlock()

	start := utils.GetTime().Now()
	self.Debug("FlushIndex: last_flush_time %v now %v",
		self.last_flush_time.Unix(),
		start.Unix())
	if start.Sub(self.last_flush_time) < 5*time.Second {
		return 0, nil
	}

	return self._FlushIndex(ctx)
}

func (self *HuntStorageManagerImpl) _FlushIndex(ctx context.Context) (int, error) {

	// Nothing to do because none of the records are dirty.
	if !self.dirty {
		return 0, nil
	}

	if atomic.LoadInt64(&self.closed) > 0 {
		return 0, nil
	}

	self.Debug("Flushing index with %v items", len(self.hunts))

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
		logger.Debug(
			"HuntDispatcher: <green>Rebuilt Hunt Index in %v for %v (%v hunts)</>",
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
		return 0, err
	}
	defer rs_writer.Close()

	// Sort hunts by hunt id so latest hunt is at the top of the
	// table.
	sort.Sort(sort.Reverse(sort.StringSlice(hunt_ids)))

	count := 0
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

			// Update the serialized representation of the object to
			// speed up flushing the index next time.
			serialized, err := json.Marshal(hunt_record.Hunt)
			if err == nil {
				hunt_record.serialized = serialized
			}
			self.hunts[hunt_id] = hunt_record
		}

		jsonl := json.Format(
			`{"HuntId":%q,"Description":%q,"Tags":%q,"Created":%q,"Started":%q,"Expires":%q,"Creator":%q,"Hunt":%q}
`,
			hunt_record.HuntId,
			hunt_record.HuntDescription,

			// Store the tags in the index so we can search for them.
			strings.Join(hunt_record.Tags, "\n"),
			hunt_record.CreateTime,
			hunt_record.StartTime,
			hunt_record.Expires,
			hunt_record.Creator,
			base64.StdEncoding.EncodeToString(hunt_record.serialized))
		rs_writer.WriteJSONL([]byte(jsonl), 1)
		count++
	}

	self.dirty = false

	return count, nil
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

func (self *HuntDispatcher) RebuildHuntIndex(
	ctx context.Context, hunt_id string, force bool) (*ordereddict.Dict, error) {

	store, ok := self.Store.(*HuntStorageManagerImpl)
	if !ok {
		return nil, utils.NotImplementedError
	}

	return store.RebuildHuntIndex(ctx, hunt_id, force)
}

// RebuildHuntIndex allows external callers to trigger an index
// rebuild operation. This is only called on demand using the
// hunt_reindex() VQL plugin.
func (self *HuntStorageManagerImpl) RebuildHuntIndex(
	ctx context.Context, hunt_id string, force bool) (*ordereddict.Dict, error) {

	if hunt_id == "" {
		refresh_stats, err := self.LoadHuntsFromDatastore(
			ctx, self.config_obj, true)
		return refresh_stats.ToDict(), err
	}

	refresh_stats := NewHuntRefreshStats("Datastore")
	self.tracker.AddRefreshStats(refresh_stats)

	launcher, err := services.GetLauncher(self.config_obj)
	if err != nil {
		return nil, err
	}

	err = self.LoadHuntObjFromDisk(
		ctx, self.config_obj, launcher,
		hunt_id, refresh_stats, force)
	return refresh_stats.ToDict(), err
}
