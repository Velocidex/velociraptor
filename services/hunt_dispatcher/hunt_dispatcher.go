/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

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

// In multi frontend deployments, each node has its own hunt
// dispatcher, initialized from the data store. On minion nodes, the
// hunt dispatcher is not allowed to write updates to the data store,
// only read them. The master's hunt dispatcher is responsible for
// maintaining the hunt state across all nodes. In order to update a
// hunt's property (e.g. TotalClientsScheduled etc), callers should
// call MutateHunt() to pass a mutation to the master.

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
)

var (
	dispatcherCurrentTimestamp = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "hunt_dispatcher_last_timestamp",
		Help: "Last timestamp of most recent hunt.",
	})
)

// The hunt dispatcher is a singlton which keeps hunt information in
// memory under lock. We can modify hunt statistics, query for
// applicable hunts etc. Hunts are flushed to disk periodically and
// read from disk when new hunts are created.
type HuntDispatcher struct {
	// This is the last timestamp of the latest hunt. At steady
	// state all clients will have run all hunts, therefore we can
	// immediately serve their forman checks by simply comparing a
	// single number.
	// NOTE: This has to be aligned to 64 bits or 32 bit builds will break
	// https://github.com/golang/go/issues/13868
	last_timestamp uint64
	config_obj     *config_proto.Config

	mu    sync.Mutex
	hunts map[string]*api_proto.Hunt
	dirty bool
}

func (self *HuntDispatcher) GetLastTimestamp() uint64 {
	return atomic.LoadUint64(&self.last_timestamp)
}

func (self *HuntDispatcher) getHunts() []*api_proto.Hunt {
	result := make([]*api_proto.Hunt, 0, len(self.hunts))
	for _, hunt := range self.hunts {
		result = append(result, hunt)
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
		return proto.Clone(hunt_obj).(*api_proto.Hunt), true
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

	return journal.PushRowsToArtifact(config_obj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("hunt_id", mutation.HuntId).
			Set("mutation", mutation)},
		"Server.Internal.HuntModification", "server", "")
}

// Modify the hunt object under lock.
func (self *HuntDispatcher) ModifyHunt(
	hunt_id string, cb func(hunt *api_proto.Hunt) error) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	if !services.GetFrontendManager().IsMaster() {
		// This is really a critical error.
		logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
		logger.Error("Unable to modify hunts on a minion node. Please use MutateHunt()")
		return errors.New("Unable to modify hunts on a minion node. Please use MutateHunt()")
	}

	hunt_obj, pres := self.hunts[hunt_id]
	if !pres {
		return errors.New("not found")
	}

	err := cb(hunt_obj)

	// Hunt is only modified if the cb return no error
	if err == nil {
		// The hunts start time could have been modified - we
		// need to update ours then.
		if hunt_obj.StartTime > self.GetLastTimestamp() {
			dispatcherCurrentTimestamp.Set(float64(hunt_obj.StartTime))
			atomic.StoreUint64(&self.last_timestamp, hunt_obj.StartTime)
		}
		self.dirty = true
	}

	return err
}

// Write the hunt stats to the data store. This is only called by the
// hunt manager and so should be concurrently safe.
func (self *HuntDispatcher) _flush_stats(config_obj *config_proto.Config) error {
	// Already locked.
	// Only do something if we are dirty.
	if !self.dirty {
		return nil
	}

	// If we are a minion frontend we never flush data - it is up
	// to the master to sync the hunts
	if !services.GetFrontendManager().IsMaster() {
		return nil
	}

	self.dirty = false

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	// Update the hunt object
	for _, hunt_obj := range self.hunts {
		hunt_path_manager := paths.NewHuntPathManager(hunt_obj.HuntId)
		err = db.SetSubject(config_obj, hunt_path_manager.Path(), hunt_obj)
		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
			logger.Error("Flushing %s to disk: %v", hunt_obj.HuntId, err)
			continue
		}
	}

	return nil
}

func (self *HuntDispatcher) Close(config_obj *config_proto.Config) {
	self.mu.Lock()
	defer self.mu.Unlock()
	atomic.SwapUint64(&self.last_timestamp, 0)

	_ = self._flush_stats(config_obj)
}

// Check for new hunts from the db. This could take a while and be
// under lock. However, while we do this we do not block the foreman
// checks.
func (self *HuntDispatcher) Refresh(config_obj *config_proto.Config) error {
	// The foreman will now skip all hunts without blocking. This
	// is OK because we will get those clients on the next foreman
	// update - the important thing is that foreman checks are not
	// blocked by this.
	last_timestamp := atomic.SwapUint64(&self.last_timestamp, 0)

	defer func() {
		dispatcherCurrentTimestamp.Set(float64(last_timestamp))
		atomic.StoreUint64(&self.last_timestamp, last_timestamp)
	}()

	self.mu.Lock()
	defer self.mu.Unlock()

	// First flush all the stats to the data store.
	err := self._flush_stats(config_obj)
	if err != nil {
		return err
	}

	// Now read all the data again.
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	hunt_path_manager := paths.NewHuntPathManager("")
	hunts, err := db.ListChildren(config_obj,
		hunt_path_manager.HuntDirectory(), 0, 1000)
	if err != nil {
		return err
	}

	for _, hunt_urn := range hunts {
		hunt_id := hunt_urn.Base()
		if !constants.HuntIdRegex.MatchString(hunt_id) {
			continue
		}

		hunt_obj := &api_proto.Hunt{}
		hunt_path_manager := paths.NewHuntPathManager(hunt_id)
		err = db.GetSubject(
			config_obj, hunt_path_manager.Path(), hunt_obj)
		if err != nil || hunt_obj.HuntId == "" {
			continue
		}

		if hunt_obj.Stats == nil {
			hunt_obj.Stats = &api_proto.HuntStats{}
		}

		// Should not really happen but if the file is
		// corrupted we skip it.
		if hunt_obj.HuntId != hunt_id {
			continue
		}

		// This hunt is newer than the last_timestamp, we need
		// to update it.
		if hunt_obj.StartTime > last_timestamp {
			last_timestamp = hunt_obj.StartTime
		}

		self.hunts[hunt_id] = hunt_obj
	}
	return nil
}

func StartHuntDispatcher(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	result := &HuntDispatcher{
		config_obj: config_obj,
		hunts:      make(map[string]*api_proto.Hunt),
	}
	services.RegisterHuntDispatcher(result)

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
			// Also listen for notifications so we can refresh as soon as
			// the hunt is started.
			notifier := services.GetNotifier()
			// This only happens at shutdown so wait until
			// we shutdown as well.
			if notifier == nil {
				<-ctx.Done()
				return
			}
			notification, cancel := notifier.ListenForNotification(
				"HuntDispatcher")
			defer cancel()

			select {
			case <-ctx.Done():
				result.Close(config_obj)
				return

			case <-notification:
				cancel()
				_ = result.Refresh(config_obj)

			case <-time.After(10 * time.Second):
				err := result.Refresh(config_obj)
				if err != nil {
					logger.Error("Unable to flush hunts: %v", err)
				}
			}
		}
	}()

	// Try to refresh the hunts table the first time. If we cant
	// we will just keep trying anyway later.
	err := result.Refresh(config_obj)
	if err != nil {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Error("Unable to Refresh hunt dispatcher: %v", err)
	}

	return nil
}
