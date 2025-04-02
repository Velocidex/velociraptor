package services

import (
	"context"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

/*
  The ExportManager is responsible for managing exports from:

  - Hunts
  - Collections
  - Notebooks

*/

type ExportType int

const (
	HuntExport = iota + 1
	FlowExport
	NotebookExport
)

type ContainerOptions struct {
	Type              ExportType
	ContainerFilename api.FSPathSpec

	// Where to write the stats object
	StatsPath api.DSPathSpec

	// Used based on the type
	HuntId     string
	FlowId     string
	ClientId   string
	NotebookId string
}

type ExportManager interface {
	SetContainerStats(
		ctx context.Context,
		config_obj *config_proto.Config,
		stats *api_proto.ContainerStats,
		opts ContainerOptions) error

	GetAvailableDownloadFiles(
		ctx context.Context,
		config_obj *config_proto.Config,
		opts ContainerOptions) (*api_proto.AvailableDownloads, error)
}

func GetExportManager(config_obj *config_proto.Config) (ExportManager, error) {
	org_manager, err := GetOrgManager()
	if err != nil {
		return nil, err
	}

	return org_manager.Services(config_obj.OrgId).ExportManager()
}
