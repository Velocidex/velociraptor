package client_monitoring

import (
	"context"
	"os"
	"sort"
	"strings"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
)

func (self *ClientEventTable) ListAvailableEventResults(
	ctx context.Context,
	in *api_proto.ListAvailableEventResultsRequest) (
	*api_proto.ListAvailableEventResultsResponse, error) {

	if in.Artifact == "" {
		return listAvailableEventArtifacts(ctx, self.config_obj, in)
	}
	return listAvailableEventTimestamps(ctx, self.config_obj, in)
}

func listAvailableEventTimestamps(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.ListAvailableEventResultsRequest) (
	*api_proto.ListAvailableEventResultsResponse, error) {

	path_manager, err := artifacts.NewArtifactPathManager(ctx,
		config_obj, in.ClientId, "", in.Artifact)
	if err != nil {
		return nil, err
	}

	result := &api_proto.ListAvailableEventResultsResponse{
		Logs: []*api_proto.AvailableEvent{
			{
				Artifact: in.Artifact,
			},
		},
	}

	timestamps, err := listAvailableEventTimestampFiles(ctx, config_obj, path_manager)
	if err != nil {
		return nil, err
	}

	result.Logs[0].RowTimestamps = timestamps

	timestamps, err = listAvailableEventTimestampFiles(
		ctx, config_obj, path_manager.Logs())
	if err != nil {
		return nil, err
	}

	result.Logs[0].LogTimestamps = timestamps

	return result, nil
}

func listAvailableEventTimestampFiles(
	ctx context.Context,
	config_obj *config_proto.Config,
	path_manager api.PathManager) ([]int32, error) {
	result := []int32{}

	reader, err := result_sets.NewTimedResultSetReader(
		ctx, config_obj, path_manager)
	if err != nil {
		return nil, err
	}

	for _, prop := range reader.GetAvailableFiles(ctx) {
		result = append(result, int32(prop.StartTime.Unix()))
	}
	return result, nil
}

func listAvailableEventArtifacts(
	ctx context.Context, config_obj *config_proto.Config,
	in *api_proto.ListAvailableEventResultsRequest) (
	*api_proto.ListAvailableEventResultsResponse, error) {

	// Figure out where all the monitoring artifacts logs are
	// stored by looking at some examples.
	exemplar := "Generic.Client.Stats"
	if in.ClientId == "" || in.ClientId == "server" {
		exemplar = "Server.Monitor.Health"
	}

	path_manager, err := artifacts.NewArtifactPathManager(ctx,
		config_obj, in.ClientId, "", exemplar)
	if err != nil {
		return nil, err
	}

	// getAllArtifacts analyses the path name from disk and adds
	// to the events list.
	seen := make(map[string]*api_proto.AvailableEvent)
	err = getAllArtifacts(ctx, config_obj, path_manager.GetRootPath(), seen)
	if err != nil {
		return nil, err
	}

	err = getAllArtifacts(ctx, config_obj,
		path_manager.Logs().GetRootPath(), seen)
	if err != nil {
		return nil, err
	}

	result := &api_proto.ListAvailableEventResultsResponse{}
	for _, item := range seen {
		result.Logs = append(result.Logs, item)
	}

	sort.Slice(result.Logs, func(i, j int) bool {
		return result.Logs[i].Artifact < result.Logs[j].Artifact
	})

	return result, nil
}

func getAllArtifacts(
	ctx context.Context,
	config_obj *config_proto.Config,
	log_path api.FSPathSpec,
	seen map[string]*api_proto.AvailableEvent) error {

	file_store_factory := file_store.GetFileStore(config_obj)

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}

	return api.Walk(file_store_factory, log_path,
		func(full_path api.FSPathSpec, info os.FileInfo) error {
			// Walking the events directory will give us
			// all the day json files. Each day json file
			// is contained in a directory structure which
			// reflects the name of the artifact, for
			// example:

			// <log_path>/Server.Monitor.Health/Prometheus/2021-08-01.json
			// Corresponds to the artifact Server.Monitor.Health/Prometheus
			if !info.IsDir() && info.Size() > 0 {
				relative_path := full_path.Dir().
					Components()[len(log_path.Components()):]
				if len(relative_path) == 0 {
					return nil
				}

				// Check if this is a valid artifact.
				artifact_base_name := relative_path[0]
				_, pres := repository.Get(ctx, config_obj, artifact_base_name)
				if !pres {
					return nil
				}

				artifact_name := strings.Join(relative_path, "/")
				_, pres = seen[artifact_name]
				if !pres {
					seen[artifact_name] = &api_proto.AvailableEvent{
						Artifact: artifact_name,
					}
				}
			}
			return nil
		})
}
