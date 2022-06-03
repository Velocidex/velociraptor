package services

import (
	"errors"
	"sync"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
)

var (
	vfs_service    VFSService
	vfs_service_mu sync.Mutex
)

func GetVFSService() (VFSService, error) {
	vfs_service_mu.Lock()
	defer vfs_service_mu.Unlock()

	if vfs_service == nil {
		return nil, errors.New("VFSService not initialized")
	}

	return vfs_service, nil
}

func RegisterVFSService(m VFSService) {
	vfs_service_mu.Lock()
	defer vfs_service_mu.Unlock()

	vfs_service = m
}

type VFSService interface {
	ListDirectory(
		config_obj *config_proto.Config,
		client_id string,
		components []string) (*api_proto.VFSListResponse, error)

	StatDirectory(
		config_obj *config_proto.Config,
		client_id string,
		vfs_components []string) (*api_proto.VFSListResponse, error)

	StatDownload(
		config_obj *config_proto.Config,
		client_id string,
		accessor string,
		path_components []string) (*flows_proto.VFSDownloadInfo, error)
}
