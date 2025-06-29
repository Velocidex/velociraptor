package exports

import (
	"context"
	"strings"
	"sync"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

type ExportManager struct {
	config_obj *config_proto.Config
}

func (self *ExportManager) GetAvailableDownloadFiles(
	ctx context.Context,
	config_obj *config_proto.Config,
	opts services.ContainerOptions) (*api_proto.AvailableDownloads, error) {
	result := &api_proto.AvailableDownloads{}
	var download_dir api.FSPathSpec

	switch opts.Type {
	case services.FlowExport:
		flow_path_manager := paths.NewFlowPathManager(opts.ClientId, opts.FlowId)
		download_dir = flow_path_manager.GetDownloadsDirectory()

	case services.HuntExport:
		hunt_path_manager := paths.NewHuntPathManager(opts.HuntId)
		download_file := hunt_path_manager.GetHuntDownloadsFile(false, "", false)
		download_dir = download_file.Dir()

	case services.NotebookExport:
		download_dir = paths.NewNotebookPathManager(opts.NotebookId).
			HtmlExport("X").Dir()

	default:
		return nil, utils.Wrap(utils.InvalidArgError, "Invalid container stat type")
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	children, err := db.ListChildren(config_obj, download_dir.AsDatastorePath())
	if err != nil {
		return nil, err
	}

	for _, child := range children {
		stats := &api_proto.ContainerStats{}
		err := db.GetSubject(config_obj, child, stats)
		if err != nil {
			continue
		}

		result.Files = append(result.Files, &api_proto.AvailableDownloadFile{
			Name:     child.Base(),
			Type:     stats.Type,
			Size:     stats.TotalCompressedBytes,
			Path:     strings.Join(stats.Components, "/"),
			Complete: stats.Hash != "",
			Stats:    stats,
		})
	}

	return result, nil
}

func (self *ExportManager) SetContainerStats(
	ctx context.Context,
	config_obj *config_proto.Config,
	stats *api_proto.ContainerStats,
	opts services.ContainerOptions) error {

	var stats_path api.DSPathSpec

	switch opts.Type {
	case services.FlowExport, services.HuntExport, services.NotebookExport:
		stats_path = opts.StatsPath
		if stats_path == nil {
			return utils.InvalidArgError
		}

	default:
		return utils.Wrap(utils.InvalidArgError, "Invalid container stat type")
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	return db.SetSubject(config_obj, stats_path, stats)
}

func NewExportManager(ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.ExportManager, error) {

	res := &ExportManager{
		config_obj: config_obj,
	}
	return res, nil
}
