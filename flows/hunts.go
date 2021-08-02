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
// Manage in memory hunt replication.  For performance, the hunts
// table is mirrored in memory and refreshed periodically. The clients
// are then compared against it on each poll and hunts are dispatched
// as needed.
package flows

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/binary"
	"path"
	"sort"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/golang/protobuf/proto"
	errors "github.com/pkg/errors"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

func GetNewHuntId() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)

	binary.BigEndian.PutUint32(buf, uint32(time.Now().Unix()))
	result := base32.HexEncoding.EncodeToString(buf)[:13]

	return constants.HUNT_PREFIX + result
}

// Backwards compatibility: Figure out the list of collected hunts
// from the hunt object's request
func FindCollectedArtifacts(
	config_obj *config_proto.Config,
	hunt *api_proto.Hunt) {
	if hunt == nil || hunt.StartRequest == nil ||
		hunt.StartRequest.Artifacts == nil {
		return
	}

	// Hunt already has artifacts list.
	if len(hunt.Artifacts) > 0 {
		return
	}

	hunt.Artifacts = hunt.StartRequest.Artifacts
	hunt.ArtifactSources = []string{}
	for _, artifact := range hunt.StartRequest.Artifacts {
		for _, source := range GetArtifactSources(
			config_obj, artifact) {
			hunt.ArtifactSources = append(
				hunt.ArtifactSources,
				path.Join(artifact, source))
		}
	}
}

func GetArtifactSources(
	config_obj *config_proto.Config,
	artifact string) []string {
	result := []string{}
	manager, err := services.GetRepositoryManager()
	if err != nil {
		return nil
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err == nil {
		artifact_obj, pres := repository.Get(config_obj, artifact)
		if pres {
			for _, source := range artifact_obj.Sources {
				result = append(result, source.Name)
			}
		}
	}
	return result
}

func CreateHunt(
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
		hunt.Expires = uint64(time.Now().Add(7*24*time.Hour).
			UTC().UnixNano() / 1000)
	}

	if hunt.Expires < hunt.CreateTime {
		return "", errors.New("Hunt expiry is in the past!")
	}

	// Set the artifacts information in the hunt object itself.
	hunt.Artifacts = hunt.StartRequest.Artifacts
	hunt.ArtifactSources = []string{}
	for _, artifact := range hunt.StartRequest.Artifacts {
		for _, source := range GetArtifactSources(
			config_obj, artifact) {
			hunt.ArtifactSources = append(
				hunt.ArtifactSources, path.Join(artifact, source))
		}
	}

	manager, err := services.GetRepositoryManager()
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
	launcher, err := services.GetLauncher()
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
	hunt.StartRequest.CompiledCollectorArgs = append(
		hunt.StartRequest.CompiledCollectorArgs, compiled...)

	// We allow our caller to determine if hunts are created in
	// the running state or the paused state.
	if hunt.State == api_proto.Hunt_UNSET {
		hunt.State = api_proto.Hunt_PAUSED

		// IF we are creating the hunt in the running state
		// set it started.
	} else if hunt.State == api_proto.Hunt_RUNNING {
		hunt.StartTime = hunt.CreateTime

		// Notify all the clients.
		notifier := services.GetNotifier()
		if notifier != nil {
			err = notifier.NotifyByRegex(config_obj, "^[Cc]\\.")
			if err != nil {
				return "", err
			}
		}
	}

	row := ordereddict.NewDict().
		Set("Timestamp", time.Now().UTC().Unix()).
		Set("Hunt", hunt)

	journal, err := services.GetJournal()
	if err != nil {
		return "", err
	}

	err = journal.PushRowsToArtifact(config_obj,
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
	err = services.GetHuntDispatcher().Refresh(config_obj)

	return hunt.HuntId, err
}

func ListHunts(config_obj *config_proto.Config, in *api_proto.ListHuntsRequest) (
	*api_proto.ListHuntsResponse, error) {

	dispatcher := services.GetHuntDispatcher()
	if dispatcher == nil {
		return nil, errors.New("Hunt dispatcher not initialized")
	}

	end := in.Count + in.Offset
	if end > 1000 {
		end = 1000
	}

	// We need to get all the active hunts so we can sort them by
	// creation time. This should be very fast because all hunts
	// are kept in memory inside the hunt dispatcher.
	items := make([]*api_proto.Hunt, 0, end)
	err := dispatcher.ApplyFuncOnHunts(
		func(hunt *api_proto.Hunt) error {
			// Only show non-archived hunts.
			if in.IncludeArchived ||
				hunt.State != api_proto.Hunt_ARCHIVED {

				// Clone the hunts so we can remove
				// them from the locked section.
				items = append(items,
					proto.Clone(hunt).(*api_proto.Hunt))
			}
			return nil
		})
	if err != nil {
		return nil, err
	}

	// Sort the hunts by creations time.
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreateTime > items[j].CreateTime
	})

	if end > uint64(len(items)) {
		end = uint64(len(items))
	}

	return &api_proto.ListHuntsResponse{
		Items: items[in.Offset:end],
	}, nil
}

func GetHunt(config_obj *config_proto.Config, in *api_proto.GetHuntRequest) (
	hunt *api_proto.Hunt, err error) {

	var hunt_obj *api_proto.Hunt

	dispatcher := services.GetHuntDispatcher()
	if dispatcher == nil {
		return nil, errors.New("Hunt dispatcher not valid")
	}

	hunt_obj, pres := dispatcher.GetHunt(in.HuntId)
	if !pres {
		return nil, errors.New("Hunt not found")
	}

	// Normalize the hunt object
	FindCollectedArtifacts(config_obj, hunt_obj)

	if hunt_obj == nil || hunt_obj.Stats == nil {
		return nil, errors.New("Not found")
	}

	hunt_obj.Stats.AvailableDownloads, _ = availableHuntDownloadFiles(config_obj, in.HuntId)

	return hunt_obj, nil
}

// availableHuntDownloadFiles returns the prepared zip downloads available to
// be fetched by the user at this moment.
func availableHuntDownloadFiles(config_obj *config_proto.Config,
	hunt_id string) (*api_proto.AvailableDownloads, error) {

	hunt_path_manager := paths.NewHuntPathManager(hunt_id)
	download_file := hunt_path_manager.GetHuntDownloadsFile(false, "")
	download_path := download_file.Dir()

	return getAvailableDownloadFiles(config_obj, download_path)
}

// This method modifies the hunt. Only the following modifications are allowed:

// 1. A hunt in the paused state can go to the running state. This
//    will update the StartTime.
// 2. A hunt in the running state can go to the Stop state
// 3. A hunt's description can be modified.
func ModifyHunt(
	ctx context.Context,
	config_obj *config_proto.Config,
	hunt_modification *api_proto.Hunt,
	user string) error {

	// We can not modify the hunt directly, instead we send a
	// mutation to the hunt manager on the master.
	mutation := &api_proto.HuntMutation{
		HuntId:      hunt_modification.HuntId,
		Description: hunt_modification.HuntDescription,
	}

	// Is the description changed?
	if hunt_modification.HuntDescription != "" {
		mutation.Description = hunt_modification.HuntDescription

		// Archive the hunt.
	} else if hunt_modification.State == api_proto.Hunt_ARCHIVED {
		mutation.State = api_proto.Hunt_ARCHIVED

		row := ordereddict.NewDict().
			Set("Timestamp", time.Now().UTC().Unix()).
			Set("HuntId", mutation.HuntId).
			Set("User", user)

		// Alert listeners that the hunt is being archived.
		journal, err := services.GetJournal()
		if err != nil {
			return err
		}

		err = journal.PushRowsToArtifact(config_obj,
			[]*ordereddict.Dict{row}, "System.Hunt.Archive",
			"server", hunt_modification.HuntId)
		if err != nil {
			return err
		}

		// We are trying to start or restart the hunt.
	} else if hunt_modification.State == api_proto.Hunt_RUNNING {

		// We allow restarting stopped hunts
		// but this may not work as intended
		// because we still have a hunt index
		// - i.e. clients that already
		// scheduled the hunt will not
		// re-schedule (whether they ran it or
		// not). Usually the most reliable way
		// to re-do a hunt is to copy it and
		// do it again.
		mutation.State = api_proto.Hunt_RUNNING
		mutation.StartTime = uint64(time.Now().UnixNano() / 1000)

		// We are trying to pause or stop the hunt.
	} else if hunt_modification.State == api_proto.Hunt_STOPPED ||
		hunt_modification.State == api_proto.Hunt_PAUSED {
		mutation.State = api_proto.Hunt_STOPPED
	}

	dispatcher := services.GetHuntDispatcher()
	err := dispatcher.MutateHunt(config_obj, mutation)
	if err != nil {
		return err
	}

	// Notify all the clients about the new hunt. New hunts are
	// not that common so notifying all the clients at once is
	// probably ok.
	notifier := services.GetNotifier()
	if notifier == nil {
		return errors.New("Notifier not ready")
	}
	return notifier.NotifyByRegex(config_obj, "^[Cc]\\.")
}
