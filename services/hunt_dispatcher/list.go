package hunt_dispatcher

import (
	"context"
	"errors"
	"path"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
)

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

func (self *HuntDispatcher) ListHunts(
	ctx context.Context, config_obj *config_proto.Config,
	in *api_proto.ListHuntsRequest) (
	*api_proto.ListHuntsResponse, error) {

	dispatcher, err := services.GetHuntDispatcher(config_obj)
	if err != nil {
		return nil, err
	}

	end := in.Count + in.Offset
	if end > 1000 {
		end = 1000
	}

	// We need to get all the active hunts so we can sort them by
	// creation time. This should be very fast because all hunts
	// are kept in memory inside the hunt dispatcher.
	items := make([]*api_proto.Hunt, 0, end)
	err = dispatcher.ApplyFuncOnHunts(
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

	dispatcher, err := services.GetHuntDispatcher(config_obj)
	if err != nil {
		return nil, err
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
	download_file := hunt_path_manager.GetHuntDownloadsFile(false, "", false)
	download_path := download_file.Dir()

	return getAvailableDownloadFiles(config_obj, download_path)
}

func getAvailableDownloadFiles(config_obj *config_proto.Config,
	download_dir api.FSPathSpec) (*api_proto.AvailableDownloads, error) {
	result := &api_proto.AvailableDownloads{}

	file_store_factory := file_store.GetFileStore(config_obj)
	files, err := file_store_factory.ListDirectory(download_dir)
	if err != nil {
		return nil, err
	}

	is_complete := func(name string) bool {
		for _, item := range files {
			ps := item.PathSpec()
			// If there is a lock file we are not done.
			if ps.Base() == name &&
				ps.Type() == api.PATH_TYPE_FILESTORE_LOCK {
				return false
			}
		}
		return true
	}

	for _, item := range files {
		ps := item.PathSpec()

		// Skip lock files
		if ps.Type() == api.PATH_TYPE_FILESTORE_LOCK {
			continue
		}

		result.Files = append(result.Files, &api_proto.AvailableDownloadFile{
			Name:     item.Name(),
			Type:     api.GetExtensionForFilestore(ps),
			Path:     ps.AsClientPath(),
			Size:     uint64(item.Size()),
			Date:     item.ModTime().UTC().Format(time.RFC3339),
			Complete: is_complete(ps.Base()),
		})
	}

	return result, nil
}
