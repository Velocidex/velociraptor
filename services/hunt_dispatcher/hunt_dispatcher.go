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
	"path"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
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
)

type HuntRecord struct {
	*api_proto.Hunt

	dirty bool

	// A serialized version of the above hunt object. If not dirty the
	// serialized version is synchronized with the hunt object.
	serialized []byte
}

// The hunt dispatcher is a singlton which keeps hunt information in
// memory under lock. We can modify hunt statistics, query for
// applicable hunts etc. Hunts are flushed to disk periodically and
// read from disk when new hunts are created.
type HuntDispatcher struct {
	config_obj *config_proto.Config

	mu sync.Mutex

	uuid int64

	// Set to true for the master's hunt dispatcher. On the master
	// node the dispatcher has more responsibility.
	I_am_master bool

	Store HuntStorageManager
}

func (self *HuntDispatcher) GetLastTimestamp() uint64 {
	return self.Store.GetLastTimestamp()
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

	// Only update the version if it is ahead.
	self.Store.ModifyHuntObject(ctx, hunt_obj.HuntId,
		func(existing_hunt *HuntRecord) services.HuntModificationAction {
			if existing_hunt.Version < hunt_obj.Version {
				existing_hunt.Hunt = hunt_obj
				return services.HuntPropagateChanges
			}
			return services.HuntUnmodified
		})

	// A hunt went into the running state - we need to participate all
	// our currently connected clients.
	_, pres = row.Get("TriggerParticipation")
	if pres {
		self.participateAllConnectedClients(ctx, config_obj, hunt_obj.HuntId)
	}

	// On the master we also write it to storage.
	if self.I_am_master {
		err = self.Store.SetHunt(ctx, hunt_obj)
		if err != nil {
			return err
		}
	}

	return nil
}

// Applies a callback on all hunts. The callback is not allowed to
// modify the hunts since it is getting a copy of the hunt object.
func (self *HuntDispatcher) ApplyFuncOnHunts(
	ctx context.Context, options services.HuntSearchOptions,
	cb func(hunt *api_proto.Hunt) error) error {
	return self.Store.ApplyFuncOnHunts(ctx, options, cb)
}

func (self *HuntDispatcher) GetHunt(
	ctx context.Context, hunt_id string) (*api_proto.Hunt, bool) {
	hunt, err := self.Store.GetHunt(ctx, hunt_id)
	if err != nil {
		return nil, false
	}

	if hunt.Stats == nil {
		hunt.Stats = &api_proto.HuntStats{}
	}

	hunt.Stats.AvailableDownloads, _ = availableHuntDownloadFiles(
		self.config_obj, hunt_id)

	// Normalize the hunt object
	FindCollectedArtifacts(ctx, self.config_obj, hunt)

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
// dispatchers about the new state. This function can only be called
// on the Master node! Other nodes may not modify any underlying data
// and must send mutations instead.
func (self *HuntDispatcher) ModifyHuntObject(
	ctx context.Context, hunt_id string,
	cb func(hunt *api_proto.Hunt) services.HuntModificationAction) services.HuntModificationAction {

	return self.Store.ModifyHuntObject(ctx, hunt_id,
		func(hunt_record *HuntRecord) services.HuntModificationAction {
			if hunt_record == nil || hunt_record.Hunt == nil {
				return services.HuntUnmodified
			}

			// Call the callback to see if we need to change this
			// hunt.
			modification := cb(hunt_record.Hunt)
			switch modification {
			case services.HuntUnmodified:
				return services.HuntUnmodified

			case services.HuntTriggerParticipation:
				// Relay the new update to all other hunt dispatchers.
				journal, err := services.GetJournal(self.config_obj)
				if err == nil {
					hunt_copy := proto.Clone(hunt_record.Hunt).(*api_proto.Hunt)

					// Make sure these are pushed out ASAP to the
					// other dispatchers.
					journal.PushRowsToArtifact(ctx, self.config_obj,
						[]*ordereddict.Dict{
							ordereddict.NewDict().
								Set("HuntId", hunt_record.HuntId).
								Set("Hunt", hunt_copy).
								Set("TriggerParticipation", true),
						},
						"Server.Internal.HuntUpdate", "server", "")
				}
				return services.HuntTriggerParticipation

			case services.HuntPropagateChanges:
				// Relay the new update to all other hunt dispatchers.
				journal, err := services.GetJournal(self.config_obj)
				if err == nil {
					hunt_copy := proto.Clone(hunt_record.Hunt).(*api_proto.Hunt)

					// Make sure these are pushed out ASAP to the
					// other dispatchers.
					journal.PushRowsToArtifact(ctx, self.config_obj,
						[]*ordereddict.Dict{
							ordereddict.NewDict().
								Set("HuntId", hunt_record.HuntId).
								Set("Hunt", hunt_copy),
						},
						"Server.Internal.HuntUpdate", "server", "")
				}
				return services.HuntPropagateChanges

			default:
				return modification
			}
		})
}

func (self *HuntDispatcher) Close(ctx context.Context) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.Store.Close(ctx)
}

func (self *HuntDispatcher) checkForExpiry(
	ctx context.Context, config_obj *config_proto.Config) {
	if self.I_am_master {
		// Check if the hunt is expired and adjust its state if so
		now := uint64(utils.GetTime().Now().UnixNano() / 1000)

		self.ApplyFuncOnHunts(ctx, services.OnlyRunningHunts,
			func(hunt_obj *api_proto.Hunt) error {
				if hunt_obj.State == api_proto.Hunt_RUNNING &&
					now > hunt_obj.Expires {

					self.MutateHunt(ctx, config_obj,
						&api_proto.HuntMutation{
							HuntId: hunt_obj.HuntId,
							State:  api_proto.Hunt_STOPPED,
							Stats:  &api_proto.HuntStats{Stopped: true},
						})
				}
				return nil
			})
	}
}

// Check for new hunts from the datastore. The master frontend will
// also flush updated hunt records to the datastore.
func (self *HuntDispatcher) Refresh(
	ctx context.Context, config_obj *config_proto.Config) error {
	self.checkForExpiry(ctx, config_obj)

	return self.Store.Refresh(ctx, config_obj)
}

func (self *HuntDispatcher) CreateHunt(
	ctx context.Context,
	config_obj *config_proto.Config,
	acl_manager vql_subsystem.ACLManager,
	hunt *api_proto.Hunt) (*api_proto.Hunt, error) {

	// Make a local copy so we can modify it safely.
	hunt, _ = proto.Clone(hunt).(*api_proto.Hunt)

	if hunt.Stats == nil {
		hunt.Stats = &api_proto.HuntStats{}
	}

	if hunt.HuntId == "" {
		hunt.HuntId = GetNewHuntId()
	}

	if hunt.StartRequest == nil || hunt.StartRequest.Artifacts == nil {
		return nil, errors.New("No artifacts to collect.")
	}

	if hunt.CreateTime == 0 {
		hunt.CreateTime = uint64(utils.GetTime().Now().UTC().UnixNano() / 1000)
	}
	if hunt.Expires == 0 {
		default_expiry := config_obj.Defaults.HuntExpiryHours
		if default_expiry == 0 {
			default_expiry = 7 * 24
		}
		hunt.Expires = uint64(utils.GetTime().Now().Add(
			time.Duration(default_expiry)*time.Hour).
			UTC().UnixNano() / 1000)
	}

	if hunt.Expires < hunt.CreateTime {
		return nil, errors.New("Hunt expiry is in the past!")
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
		return nil, err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, err
	}

	// Compile the start request and store it in the hunt. We will
	// use this compiled version to launch all other flows from
	// this hunt rather than re-compile the artifact each
	// time. This ensures that if the artifact definition is
	// changed after this point, the hunt will continue to
	// schedule consistent VQL on the clients.
	launcher, err := services.GetLauncher(config_obj)
	if err != nil {
		return nil, err
	}

	compiled, err := launcher.CompileCollectorArgs(
		ctx, config_obj, acl_manager, repository,
		services.CompilerOptions{
			ObfuscateNames: true,
		},
		hunt.StartRequest)
	if err != nil {
		return nil, err
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
		Set("Timestamp", utils.GetTime().Now().UTC().Unix()).
		Set("Hunt", hunt)

	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return nil, err
	}

	err = journal.PushRowsToArtifact(ctx, config_obj,
		[]*ordereddict.Dict{row}, "System.Hunt.Creation",
		"server", hunt.HuntId)
	if err != nil {
		return nil, err
	}

	err = self.Store.SetHunt(ctx, hunt)
	if err != nil {
		return nil, err
	}

	// Trigger a refresh of the hunt dispatcher. This guarantees that
	// fresh data will be read in subsequent ListHunt() calls and the
	// GUI will show the new hunt immediately.
	return hunt, self.Store.FlushIndex(ctx)
}

func NewHuntDispatcher(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.IHuntDispatcher, error) {

	service := &HuntDispatcher{
		config_obj:  config_obj,
		uuid:        utils.GetGUID(),
		I_am_master: services.IsMaster(config_obj),
		Store:       NewHuntStorageManagerImpl(config_obj),
	}

	err := service.Store.Refresh(ctx, config_obj)
	if err != nil {
		return nil, err
	}

	// flush the hunts periodically
	wg.Add(1)
	go func() {
		defer wg.Done()

		refresh := int64(60)
		if config_obj.Defaults != nil &&
			config_obj.Defaults.HuntDispatcherRefreshSec > 0 {
			refresh = config_obj.Defaults.HuntDispatcherRefreshSec
		}

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
				// Give at most 10 seconds for shutdown.
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				service.Close(ctx)
				return

			case <-time.After(utils.Jitter(time.Duration(refresh) * time.Second)):
				// Re-read the hunts from the data store.
				err := service.Refresh(ctx, config_obj)
				if err != nil {
					logger.Error("Unable to sync hunts: %v", err)
				}
			}
		}
	}()

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

	binary.BigEndian.PutUint32(buf, uint32(utils.GetTime().Now().Unix()))
	result := base32.HexEncoding.EncodeToString(buf)[:13]

	return constants.HUNT_PREFIX + result
}

func init() {
	json.RegisterCustomEncoder(&api_proto.Hunt{}, json.MarshalHuntProtobuf)
}
