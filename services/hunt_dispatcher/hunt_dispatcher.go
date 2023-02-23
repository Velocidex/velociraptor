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
	"crypto/rand"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"path"
	"strings"
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
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

var (
	dispatcherCurrentTimestamp = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "hunt_dispatcher_last_timestamp",
		Help: "Last timestamp of most recent hunt.",
	})

	Clock utils.Clock = &utils.RealClock{}
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

	// Set to true for the master's hunt dispatcher. On the master
	// node the dispatcher has more responsibility.
	I_am_master bool
}

func (self *HuntDispatcher) GetLastTimestamp() uint64 {
	return atomic.LoadUint64(&self.last_timestamp)
}

// When a new hunt is started, we need to inform the hunt manager
// about all clients currently directly connected to us. The hunt
// manager may decide to schedule the hunt on these clients.
func (self *HuntDispatcher) participateAllConnectedClients(
	ctx context.Context,
	config_obj *config_proto.Config, hunt_id string) error {

	notifier, err := services.GetNotifier(config_obj)
	if err != nil {
		return err
	}
	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return err
	}

	for _, c := range notifier.ListClients() {
		if !strings.HasPrefix(c, "C.") {
			continue
		}

		// Notify the hunt manager about the new client
		journal.PushRowsToArtifactAsync(ctx, config_obj,
			ordereddict.NewDict().
				Set("HuntId", hunt_id).
				Set("ClientId", c),
			"System.Hunt.Participation")
	}

	return nil
}

func (self *HuntDispatcher) ProcessUpdate(
	ctx context.Context,
	config_obj *config_proto.Config,
	row *ordereddict.Dict) error {

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

	// Only update the version if it is ahead.
	self.mu.Lock()
	existing_hunt, pres := self.hunts[hunt_obj.HuntId]

	if pres && existing_hunt.Version < hunt_obj.Version {
		self.hunts[hunt_obj.HuntId] = &HuntRecord{Hunt: hunt_obj}
	}
	self.mu.Unlock()

	// A hunt went into the running state - we need to participate all
	// our currently connected clients.
	_, pres = row.Get("TriggerParticipation")
	if pres {
		self.participateAllConnectedClients(ctx, config_obj, hunt_obj.HuntId)
	}

	// On the master we also write it to storage.
	if self.I_am_master {
		hunt_path_manager := paths.NewHuntPathManager(hunt_obj.HuntId)
		db, err := datastore.GetDB(config_obj)
		if err != nil {
			return err
		}

		err = db.SetSubjectWithCompletion(
			config_obj, hunt_path_manager.Path(), hunt_obj, nil)
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
	var hunts []*api_proto.Hunt
	self.mu.Lock()
	for _, h := range self.getHunts() {
		hunts = append(hunts, proto.Clone(h).(*api_proto.Hunt))
	}
	self.mu.Unlock()

	// Read only copy for callback
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
	hunt_obj, pres := self.hunts[hunt_id]
	if !pres || hunt_obj == nil {
		self.mu.Unlock()
		return nil, false
	}

	// Make a copy of the hunt object so we can update it safely
	hunt := proto.Clone(hunt_obj.Hunt).(*api_proto.Hunt)
	self.mu.Unlock()

	if hunt.Stats == nil {
		hunt.Stats = &api_proto.HuntStats{}
	}

	hunt.Stats.AvailableDownloads, _ = availableHuntDownloadFiles(
		self.config_obj, hunt_id)
	return hunt, true
}

// This is called by the local server to mutate the hunt
// object. Mutations include increasing the number of clients
// assigned, completed etc. These mutations may happen very frequently
// and so we do not want to flush them to disk immediately. Instead we
// push the mutations to the master node's hunt manager, where they
// will be applied on the master node. Eventually these will end up in
// the filesystem and possibly refreshed into this dispatcher.
// Therefore, writers may write mutations and expect they take an
// unspecified time to appear in the hunt details.
func (self *HuntDispatcher) MutateHunt(
	ctx context.Context, config_obj *config_proto.Config,
	mutation *api_proto.HuntMutation) error {
	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return err
	}

	journal.PushRowsToArtifactAsync(ctx, config_obj,
		ordereddict.NewDict().
			Set("hunt_id", mutation.HuntId).
			Set("mutation", mutation),
		"Server.Internal.HuntModification")

	return nil
}

// Modify the hunt object under lock and also inform all other
// dispatchers about the new state.
func (self *HuntDispatcher) ModifyHuntObject(
	ctx context.Context, hunt_id string,
	cb func(hunt *api_proto.Hunt) services.HuntModificationAction) services.HuntModificationAction {

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)

	if !self.I_am_master {
		// This is really a critical error.
		logger.Error("Unable to modify hunts on a minion node. Please use MutateHunt()")
		return services.HuntUnmodified
	}

	self.mu.Lock()
	hunt_obj, pres := self.hunts[hunt_id]
	if !pres {
		self.mu.Unlock()
		return services.HuntUnmodified
	}

	// Update the hunt version
	hunt_obj.Hunt.Version = Clock.Now().UnixNano()

	// Call the callback to see if we need to change this hunt.
	modification := cb(hunt_obj.Hunt)
	switch modification {
	case services.HuntUnmodified:
		self.mu.Unlock()

	case services.HuntTriggerParticipation:
		// It is still modified so make sure to write it eventually.
		hunt_obj.dirty = true

		hunt_obj_copy := proto.Clone(hunt_obj.Hunt).(*api_proto.Hunt)
		self.mu.Unlock()

		// Relay the new update to all other hunt dispatchers.
		journal, err := services.GetJournal(self.config_obj)
		if err == nil {
			// Make sure these are pushed out ASAP to the other
			// dispatchers.
			journal.PushRowsToArtifact(ctx, self.config_obj,
				[]*ordereddict.Dict{
					ordereddict.NewDict().
						Set("HuntId", hunt_id).
						Set("Hunt", hunt_obj_copy).
						Set("TriggerParticipation", true),
				},
				"Server.Internal.HuntUpdate", "server", "")
		}

	case services.HuntPropagateChanges:
		// It is still modified so make sure to write it eventually.
		hunt_obj.dirty = true

		hunt_obj_copy := proto.Clone(hunt_obj.Hunt).(*api_proto.Hunt)
		self.mu.Unlock()

		// Relay the new update to all other hunt dispatchers.
		journal, err := services.GetJournal(self.config_obj)
		if err == nil {
			// Make sure these are pushed out ASAP to the other
			// dispatchers.
			journal.PushRowsToArtifact(ctx, self.config_obj,
				[]*ordereddict.Dict{
					ordereddict.NewDict().
						Set("HuntId", hunt_id).
						Set("Hunt", hunt_obj_copy),
				},
				"Server.Internal.HuntUpdate", "server", "")
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

		err = db.SetSubjectWithCompletion(self.config_obj,
			hunt_path_manager.Path(), hunt_obj_copy, nil)
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
			if self.I_am_master {
				db.SetSubjectWithCompletion(
					config_obj, request.Path, old_hunt_obj, nil)
			}
			continue
		}

		// Maintain the last timestamp as the latest hunt start time.
		last_timestamp := self.GetLastTimestamp()
		if hunt_obj.StartTime > last_timestamp {
			atomic.StoreUint64(&self.last_timestamp, hunt_obj.StartTime)
			dispatcherCurrentTimestamp.Set(float64(last_timestamp))
		}
		self.hunts[hunt_id] = &HuntRecord{Hunt: hunt_obj}
	}

	return nil
}

func (self *HuntDispatcher) CreateHunt(
	ctx context.Context,
	config_obj *config_proto.Config,
	acl_manager vql_subsystem.ACLManager,
	hunt *api_proto.Hunt) (string, error) {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return "", err
	}

	if hunt.Stats == nil {
		hunt.Stats = &api_proto.HuntStats{}
	}

	if hunt.HuntId == "" {
		hunt.HuntId = GetNewHuntId()
	}

	if hunt.StartRequest == nil || hunt.StartRequest.Artifacts == nil {
		return "", errors.New("No artifacts to collect.")
	}

	hunt.CreateTime = uint64(time.Now().UTC().UnixNano() / 1000)
	if hunt.Expires == 0 {
		default_expiry := config_obj.Defaults.HuntExpiryHours
		if default_expiry == 0 {
			default_expiry = 7 * 24
		}
		hunt.Expires = uint64(time.Now().Add(
			time.Duration(default_expiry)*time.Hour).
			UTC().UnixNano() / 1000)
	}

	if hunt.Expires < hunt.CreateTime {
		return "", errors.New("Hunt expiry is in the past!")
	}

	// Set the artifacts information in the hunt object itself.
	hunt.Artifacts = hunt.StartRequest.Artifacts
	hunt.ArtifactSources = []string{}
	for _, artifact := range hunt.StartRequest.Artifacts {
		for _, source := range GetArtifactSources(ctx, config_obj, artifact) {
			hunt.ArtifactSources = append(
				hunt.ArtifactSources, path.Join(artifact, source))
		}
	}

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return "", err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return "", err
	}

	// Compile the start request and store it in the hunt. We will
	// use this compiled version to launch all other flows from
	// this hunt rather than re-compile the artifact each
	// time. This ensures that if the artifact definition is
	// changed after this point, the hunt will continue to
	// schedule consistent VQL on the clients.
	launcher, err := services.GetLauncher(config_obj)
	if err != nil {
		return "", err
	}

	compiled, err := launcher.CompileCollectorArgs(
		ctx, config_obj, acl_manager, repository,
		services.CompilerOptions{
			ObfuscateNames: true,
		},
		hunt.StartRequest)
	if err != nil {
		return "", err
	}

	// Set the collection ID already on the hunt request - all flows
	// from this hunt will have the same flow id.
	hunt.StartRequest.FlowId = utils.CreateFlowIdFromHuntId(hunt.HuntId)
	hunt.StartRequest.CompiledCollectorArgs = append(
		hunt.StartRequest.CompiledCollectorArgs, compiled...)
	hunt.StartRequest.Creator = hunt.Creator

	// We allow our caller to determine if hunts are created in
	// the running state or the paused state.
	if hunt.State == api_proto.Hunt_UNSET {
		hunt.State = api_proto.Hunt_PAUSED

		// IF we are creating the hunt in the running state
		// set it started.
	} else if hunt.State == api_proto.Hunt_RUNNING {
		hunt.StartTime = hunt.CreateTime
	}

	row := ordereddict.NewDict().
		Set("Timestamp", time.Now().UTC().Unix()).
		Set("Hunt", hunt)

	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return "", err
	}

	err = journal.PushRowsToArtifact(ctx, config_obj,
		[]*ordereddict.Dict{row}, "System.Hunt.Creation",
		"server", hunt.HuntId)
	if err != nil {
		return "", err
	}

	hunt_path_manager := paths.NewHuntPathManager(hunt.HuntId)
	err = db.SetSubject(config_obj, hunt_path_manager.Path(), hunt)
	if err != nil {
		return "", err
	}

	// Trigger a refresh of the hunt dispatcher. This guarantees
	// that fresh data will be read in subsequent ListHunt()
	// calls.
	hunt_dispatcher, err := services.GetHuntDispatcher(config_obj)
	if err != nil {
		return "", err
	}
	return hunt.HuntId, hunt_dispatcher.Refresh(config_obj)
}

func NewHuntDispatcher(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.IHuntDispatcher, error) {

	service := &HuntDispatcher{
		config_obj:  config_obj,
		hunts:       make(map[string]*HuntRecord),
		uuid:        utils.GetGUID(),
		I_am_master: services.IsMaster(config_obj),
	}

	// flush the hunts every 10 seconds.
	wg.Add(1)
	go func() {
		defer wg.Done()

		// On the client we register a dummy dispatcher since
		// there is nothing to sync from.
		if config_obj.Datastore == nil {
			return
		}

		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Info("<green>Starting</> Hunt Dispatcher Service for %v.",
			services.GetOrgName(config_obj))

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

	return service, journal.WatchQueueWithCB(ctx, config_obj, wg,
		"Server.Internal.HuntUpdate", "HuntDispatcher",
		service.ProcessUpdate)
}

var (
	NextHuntIdForTests string
)

func SetHuntIdForTests(id string) {
	NextHuntIdForTests = id
}

func GetNewHuntId() string {
	if NextHuntIdForTests != "" {
		result := NextHuntIdForTests
		NextHuntIdForTests = ""
		return result
	}

	buf := make([]byte, 8)
	_, _ = rand.Read(buf)

	binary.BigEndian.PutUint32(buf, uint32(time.Now().Unix()))
	result := base32.HexEncoding.EncodeToString(buf)[:13]

	return constants.HUNT_PREFIX + result
}

func init() {
	json.RegisterCustomEncoder(&api_proto.Hunt{}, json.MarshalHuntProtobuf)
}
