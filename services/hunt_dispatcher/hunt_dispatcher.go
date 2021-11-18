/*
   Velociraptor - Digging Deeper
   Copyright (C) 2021 Velocidex.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package hunt_dispatcher

// The hunt dispatcher is a local in memory cache of current active
// hunts. As clients check in to the frontend, the server makes sure
// there are no outstanding hunts for that client, and this needs to
// be in memory for quick access. The hunt dispatcher refreshes the
// hunt list periodically from the data store to receive fresh data.

// In multi frontend deployments, each node (master or minion) has its
// own hunt dispatcher, initialized from the data store. On minion
// nodes, the hunt dispatcher is not allowed to write updates to the
// data store, only read them.

// The master's hunt dispatcher is responsible for maintaining the
// hunt state across all nodes. In order to update a hunt's property
// (e.g. TotalClientsScheduled etc), callers should call MutateHunt()
// on their local node to send a mutation to the master, which will
// actually update the hunt state.

// As the hunt manager (singleton running on the master) updates the
// hunt record, it sends the new record to the
// Server.Internal.HuntUpdate queue, where all hunt dispatchers will
// receive it and update their internal state. The hunt dispatcher on
// the master will also write the new record to the data store.

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	dispatcherCurrentTimestamp = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "hunt_dispatcher_last_timestamp",
		Help: "Last timestamp of most recent hunt.",
	})
)

type HuntRecord struct {
	*api_proto.Hunt

	dirty bool
}

// The hunt dispatcher is a singlton which keeps hunt information in
// memory under lock. We can modify hunt statistics, query for
// applicable hunts etc. Hunts are flushed to disk periodically and
// read from disk when new hunts are created.
type HuntDispatcher struct {
	// This is the last timestamp of the latest hunt. At steady
	// state all clients will have run all hunts, therefore we can
	// immediately serve their foreman checks by simply comparing a
	// single number.
	// NOTE: This has to be aligned to 64 bits or 32 bit builds will break
	// https://github.com/golang/go/issues/13868
	last_timestamp uint64
	config_obj     *config_proto.Config

	mu    sync.Mutex
	hunts map[string]*HuntRecord

	uuid int64
}

func (self *HuntDispatcher) GetLastTimestamp() uint64 {
	return atomic.LoadUint64(&self.last_timestamp)
}

func (self *HuntDispatcher) ProcessUpdate(
	ctx context.Context,
	config_obj *config_proto.Config,
	row *ordereddict.Dict) error {

	// Ignore messages we sent ourselves.
	from, pres := row.GetInt64("From")
	if !pres || from == self.uuid {
		return nil
	}

	hunt_any, pres := row.Get("Hunt")
	if !pres {
		return nil
	}

	serialized, err := json.Marshal(hunt_any)
	if err != nil {
		return err
	}

	hunt_obj := &api_proto.Hunt{}
	err = protojson.Unmarshal(serialized, hunt_obj)
	if err != nil {
		return err
	}

	// The hunts start time could have been modified - we need to
	// update ours then (and also the metrics).
	if hunt_obj.StartTime > self.GetLastTimestamp() {
		dispatcherCurrentTimestamp.Set(float64(hunt_obj.StartTime))
		atomic.StoreUint64(&self.last_timestamp, hunt_obj.StartTime)
	}

	// Update the last time.
	self.mu.Lock()
	self.hunts[hunt_obj.HuntId] = &HuntRecord{Hunt: hunt_obj}
	self.mu.Unlock()

	// On the master we also write it to storage.
	if services.GetFrontendManager().IsMaster() {
		hunt_path_manager := paths.NewHuntPathManager(hunt_obj.HuntId)
		db, err := datastore.GetDB(config_obj)
		if err != nil {
			return err
		}

		err = db.SetSubject(config_obj, hunt_path_manager.Path(), hunt_obj)
		if err != nil {
			return fmt.Errorf("Flushing hunt update %s to disk: %w", hunt_obj.HuntId, err)
		}
	}

	return nil
}

func (self *HuntDispatcher) getHunts() []*api_proto.Hunt {
	result := make([]*api_proto.Hunt, 0, len(self.hunts))
	for _, hunt := range self.hunts {
		result = append(result, hunt.Hunt)
	}

	return result
}

// Applies a callback on all hunts. The callback is not allowed to
// modify the hunts.
func (self *HuntDispatcher) ApplyFuncOnHunts(
	cb func(hunt *api_proto.Hunt) error) error {

	// Take a snapshot of the hunts list.
	self.mu.Lock()
	hunts := self.getHunts()
	self.mu.Unlock()

	for _, hunt := range hunts {
		err := cb(hunt)
		if err != nil {
			return err
		}
	}

	return nil
}

func (self *HuntDispatcher) GetHunt(hunt_id string) (*api_proto.Hunt, bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	hunt_obj, pres := self.hunts[hunt_id]
	if pres {
		return proto.Clone(hunt_obj.Hunt).(*api_proto.Hunt), true
	}

	return nil, false
}

func (self *HuntDispatcher) MutateHunt(
	config_obj *config_proto.Config,
	mutation *api_proto.HuntMutation) error {
	journal, err := services.GetJournal()
	if err != nil {
		return err
	}

	journal.PushRowsToArtifactAsync(config_obj,
		ordereddict.NewDict().
			Set("hunt_id", mutation.HuntId).
			Set("mutation", mutation),
		"Server.Internal.HuntModification")

	return nil
}

// Modify the hunt object under lock and also inform all other
// dispatchers about the new state.
func (self *HuntDispatcher) ModifyHunt(
	hunt_id string,
	cb func(hunt *api_proto.Hunt) services.HuntModificationAction) services.HuntModificationAction {

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	if !services.GetFrontendManager().IsMaster() {
		// This is really a critical error.
		logger.Error("Unable to modify hunts on a minion node. Please use MutateHunt()")
		return services.HuntUnmodified
	}

	self.mu.Lock()
	hunt_obj, pres := self.hunts[hunt_id]
	if !pres {
		return services.HuntUnmodified
	}

	hunt_obj.Hunt.Version = time.Now().UnixNano()

	// Call the callback to see if we need to change this hunt.
	modification := cb(hunt_obj.Hunt)
	switch modification {
	case services.HuntUnmodified:
		self.mu.Unlock()

	case services.HuntPropagateChanges:
		// It is still modified so make sure to write it eventually.
		hunt_obj.dirty = true

		hunt_obj_copy := proto.Clone(hunt_obj.Hunt).(*api_proto.Hunt)
		self.mu.Unlock()

		// Relay the new update to all other hunt dispatchers.
		journal, err := services.GetJournal()
		if err == nil {
			journal.PushRowsToArtifactAsync(self.config_obj,
				ordereddict.NewDict().
					Set("HuntId", hunt_id).
					Set("From", self.uuid).
					Set("Hunt", hunt_obj_copy),
				"Server.Internal.HuntUpdate")
		}

	case services.HuntFlushToDatastore:
		hunt_obj.dirty = true

		hunt_obj_copy := proto.Clone(hunt_obj.Hunt).(*api_proto.Hunt)
		self.mu.Unlock()

		hunt_path_manager := paths.NewHuntPathManager(hunt_id)
		db, err := datastore.GetDB(self.config_obj)
		if err != nil {
			return services.HuntUnmodified
		}

		err = db.SetSubject(self.config_obj,
			hunt_path_manager.Path(), hunt_obj_copy)
		if err != nil {
			logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
			logger.Error("Flushing %s to disk: %v", hunt_obj_copy, err)
			return services.HuntUnmodified
		}

	case services.HuntFlushToDatastoreAsync:
		hunt_obj.dirty = true
		self.mu.Unlock()

	default:
		self.mu.Unlock()
	}

	return modification
}

func (self *HuntDispatcher) Close(config_obj *config_proto.Config) {
	self.mu.Lock()
	defer self.mu.Unlock()

	atomic.SwapUint64(&self.last_timestamp, 0)
}

// Check for new hunts from the datastore. The master frontend will
// also flush updated hunt records to the datastore.
func (self *HuntDispatcher) Refresh(config_obj *config_proto.Config) error {
	// Now read all the data again from the data store.
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	hunt_path_manager := paths.NewHuntPathManager("")
	hunts, err := db.ListChildren(config_obj, hunt_path_manager.HuntDirectory())
	if err != nil {
		return err
	}

	requests := make([]*datastore.MultiGetSubjectRequest, 0, len(hunts))
	for _, hunt_path := range hunts {
		hunt_id := hunt_path.Base()
		if !constants.HuntIdRegex.MatchString(hunt_id) {
			continue
		}

		requests = append(requests, &datastore.MultiGetSubjectRequest{
			Path:    paths.NewHuntPathManager(hunt_id).Path(),
			Message: &api_proto.Hunt{},
			Data:    hunt_id,
		})
	}

	err = datastore.MultiGetSubject(config_obj, requests)
	if err != nil {
		return err
	}

	// Now merge the database entries with the current in memory set.
	self.mu.Lock()
	defer self.mu.Unlock()

	for _, request := range requests {
		hunt_id := request.Data.(string)
		hunt_obj, ok := request.Message.(*api_proto.Hunt)
		if !ok {
			continue
		}

		if request.Err != nil || hunt_obj.HuntId != hunt_id {
			continue
		}

		old_hunt_obj, pres := self.hunts[hunt_id]
		if pres && old_hunt_obj.Version >= hunt_obj.Version {
			// The in memory copy is newer than the stored version,
			// Master node will synchronize
			if services.GetFrontendManager().IsMaster() {
				db.SetSubject(config_obj, request.Path, old_hunt_obj)
			}
			continue
		}

		// Maintain the last timestamp as the latest hunt start time.
		if hunt_obj.StartTime > self.last_timestamp {
			self.last_timestamp = hunt_obj.StartTime
			dispatcherCurrentTimestamp.Set(float64(self.last_timestamp))
		}

		self.hunts[hunt_id] = &HuntRecord{Hunt: hunt_obj}
	}
	return nil
}

func StartHuntDispatcher(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	service := &HuntDispatcher{
		config_obj: config_obj,
		hunts:      make(map[string]*HuntRecord),
		uuid:       utils.GetGUID(),
	}
	services.RegisterHuntDispatcher(service)

	// flush the hunts every 10 seconds.
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer services.RegisterHuntDispatcher(nil)

		// On the client we register a dummy dispatcher since
		// there is nothing to sync from.
		if config_obj.Datastore == nil {
			return
		}

		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Info("<green>Starting</> Hunt Dispatcher Service.")

		for {
			select {
			case <-ctx.Done():
				service.Close(config_obj)
				return

			case <-time.After(10 * time.Second):
				// Re-read the hunts from the data store.
				err := service.Refresh(config_obj)
				if err != nil {
					logger.Error("Unable to sync hunts: %v", err)
				}
			}
		}
	}()

	// Try to refresh the hunts table the first time. If we cant
	// we will just keep trying anyway later.
	err := service.Refresh(config_obj)
	if err != nil {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Error("Unable to Refresh hunt dispatcher: %v", err)
	}

	return journal.WatchQueueWithCB(ctx, config_obj, wg,
		"Server.Internal.HuntUpdate", service.ProcessUpdate)
}

func init() {
	json.RegisterCustomEncoder(&api_proto.Hunt{}, json.MarshalHuntProtobuf)
}
