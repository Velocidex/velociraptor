package reporting

import (
	"strings"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

func GetAvailableDownloadFiles(config_obj *config_proto.Config,
	download_dir api.FSPathSpec) (*api_proto.AvailableDownloads, error) {
	result := &api_proto.AvailableDownloads{}

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
