package hunt_dispatcher

import (
	"context"
	"path"
	"sort"

	"google.golang.org/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

// Backwards compatibility: Figure out the list of collected hunts
// from the hunt object's request
func FindCollectedArtifacts(
	ctx context.Context, config_obj *config_proto.Config,
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
		for _, source := range GetArtifactSources(ctx, config_obj, artifact) {
			hunt.ArtifactSources = append(
				hunt.ArtifactSources,
				path.Join(artifact, source))
		}
	}
}

// This function is deprecated.
func (self *HuntDispatcher) ListHunts(
	ctx context.Context, config_obj *config_proto.Config,
	in *api_proto.ListHuntsRequest) (
	*api_proto.ListHuntsResponse, error) {

	end := in.Count + in.Offset
	if end > 1000 {
		end = 1000
	}

	// We need to get all the active hunts so we can sort them by
	// creation time. This should be very fast because all hunts
	// are kept in memory inside the hunt dispatcher.
	items := make([]*api_proto.Hunt, 0, end)
	err := self.ApplyFuncOnHunts(ctx, services.AllHunts,
		func(hunt *api_proto.Hunt) error {
			if in.UserFilter != "" &&
				in.UserFilter != hunt.Creator {
				return nil
			}

			// Only show non-archived hunts.
			if in.IncludeArchived ||
				hunt.State != api_proto.Hunt_ARCHIVED {

				// Clone the hunts so we can remove
				// them from the locked section.
				clone := proto.Clone(hunt).(*api_proto.Hunt)
				if clone.StartRequest != nil {
					clone.StartRequest.CompiledCollectorArgs = nil
				}

				items = append(items, clone)
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

// availableHuntDownloadFiles returns the prepared zip downloads available to
// be fetched by the user at this moment.
func availableHuntDownloadFiles(
	ctx context.Context, config_obj *config_proto.Config,
	hunt_id string) (*api_proto.AvailableDownloads, error) {

	export_manager, err := services.GetExportManager(config_obj)
	if err != nil {
		return nil, err
	}

	return export_manager.GetAvailableDownloadFiles(ctx, config_obj,
		services.ContainerOptions{
			Type:   services.HuntExport,
			HuntId: hunt_id,
		})
}
