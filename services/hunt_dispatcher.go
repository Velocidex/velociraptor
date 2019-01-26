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
package services

import (
	"errors"
	"path"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/logging"
)

var (
	global_hunt_dispatcher *HuntDispatcher
	hunt_id_regex          = regexp.MustCompile(`^H\.[^.]+$`)
)

func GetHuntDispatcher() *HuntDispatcher {
	if global_hunt_dispatcher == nil {
		panic("Global hunt dispatcher has not been initialized yet!")
	}

	return global_hunt_dispatcher
}

// The hunt dispatcher is a singlton which keeps hunt information in
// memory under lock. We can modify hunt statistics, query for
// applicable hunts etc. Hunts are flushed to disk periodically and
// read from disk when new hunts are created.

// Note: Hunt information is broken into two:
// 1. The hunt details themselves are modified by the GUI.
// 2. The hunt stats are modified by the hunt manager.

// The two are stored in different objects in the data store.
type HuntDispatcher struct {
	mu         sync.Mutex
	config_obj *api_proto.Config

	// This is the last timestamp of the latest hunt. At steady
	// state all clients will have run all hunts, therefore we can
	// immediately serve their forman checks by simply comparing a
	// single number.
	last_timestamp uint64
	hunts          map[string]*api_proto.Hunt
	done           chan bool
	dirty          bool
}

func (self *HuntDispatcher) GetLastTimestamp() uint64 {
	return atomic.LoadUint64(&self.last_timestamp)
}

// Applies a callback on all hunts. Note that the entire dispatcher is
// locked while this function is running so it should be quick. It is
// not allowed to modify the hunts.
func (self *HuntDispatcher) ApplyFuncOnHunts(
	cb func(hunt *api_proto.Hunt) error) error {

	self.mu.Lock()
	defer self.mu.Unlock()

	for _, hunt := range self.hunts {
		err := cb(hunt)
		if err != nil {
			return err
		}
	}

	return nil
}

// Modify the hunt object under lock.
func (self *HuntDispatcher) ModifyHunt(
	hunt_id string, cb func(hunt *api_proto.Hunt) error) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	hunt_obj, pres := self.hunts[path.Base(hunt_id)]
	if !pres {
		return errors.New("not found")
	}

	err := cb(hunt_obj)

	// The hunts start time could have been modified - we need to
	// update ours then.
	if hunt_obj.StartTime > self.GetLastTimestamp() {
		atomic.StoreUint64(&self.last_timestamp, hunt_obj.StartTime)
	}

	self.dirty = true

	return err
}

// Write the hunt stats to the data store. This is only called by the
// hunt manager and so should be concurrently safe.
func (self *HuntDispatcher) _flush_stats() error {
	// Only do something if we are dirty.
	if !self.dirty {
		return nil
	}

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	for _, hunt_obj := range self.hunts {
		err = db.SetSubject(
			self.config_obj,
			hunt_obj.HuntId+"/stats", hunt_obj.Stats)
		if err != nil {
			logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
			logger.Error("Flushing %s to disk: %v", hunt_obj.HuntId, err)
			continue
		}
	}

	self.dirty = false

	return nil
}

func (self *HuntDispatcher) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()
	atomic.SwapUint64(&self.last_timestamp, 0)

	self._flush_stats()
	close(self.done)
}

// Check for new hunts from the db. This could take a while and be
// under lock. However, while we do this we do not block the foreman
// checks.
func (self *HuntDispatcher) Refresh() error {
	// The foreman will now skip all hunts without blocking. This
	// is OK because we will get those clients on the next foreman
	// update.
	atomic.StoreUint64(&self.last_timestamp, 0)

	var last_timestamp uint64
	defer func() {
		atomic.StoreUint64(&self.last_timestamp, last_timestamp)
	}()

	self.mu.Lock()
	defer self.mu.Unlock()

	// First flush all the stats to the data store.
	err := self._flush_stats()
	if err != nil {
		return err
	}

	// Now read all the data again.
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	hunts, err := db.ListChildren(self.config_obj, constants.HUNTS_URN, 0, 1000)
	if err != nil {
		return err
	}

	for _, hunt_urn := range hunts {
		hunt_id := path.Base(hunt_urn)

		if !hunt_id_regex.MatchString(hunt_id) {
			continue
		}

		hunt_obj := &api_proto.Hunt{}
		err = db.GetSubject(self.config_obj, hunt_urn, hunt_obj)
		if err != nil {
			continue
		}

		// Re-read the stats into the hunt object.
		hunt_stats := &api_proto.HuntStats{}
		err := db.GetSubject(self.config_obj, hunt_urn+"/stats", hunt_stats)
		if err == nil {
			hunt_obj.Stats = hunt_stats
		}

		if path.Base(hunt_obj.HuntId) != hunt_id {
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

func startHuntDispatcher(config_obj *api_proto.Config) (*HuntDispatcher, error) {
	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("Starting hunt dispatcher.")

	result := &HuntDispatcher{
		config_obj: config_obj,
		hunts:      make(map[string]*api_proto.Hunt),
		done:       make(chan bool),
	}

	// flush the hunts every 10 seconds.
	go func() {
		for {
			select {
			case <-result.done:
				return

			case <-time.After(10 * time.Second):
				result.Refresh()
			}
		}
	}()

	err := result.Refresh()
	if err != nil {
		return nil, err
	}
	global_hunt_dispatcher = result

	return result, nil
}
