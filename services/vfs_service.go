package services

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
)

type VFSPartition struct {
	StartIdx uint64 `json:"start_idx"`
	EndIdx   uint64 `json:"end_idx"`
}

// This is the type of rows sent in the
// System.VFS.ListDirectory/Listing artifact. The VFS service will
// parse them and write to the datastore.
type VFSListRow struct {
	FullPath   string            `json:"FullPath"`
	OSPath     *accessors.OSPath `json:"OSPath"`
	Components []string          `json:"Components"`
	Accessor   string            `json:"Accessor"`
	Data       *ordereddict.Dict `json:"Data"`
	Stats      *VFSPartition     `json:"Stats"`
	Name       string            `json:"Name"`
	Size       int64             `json:"Size"`
	Mode       string            `json:"Mode"`
	Mtime      time.Time         `json:"mtime"`
	Atime      time.Time         `json:"atime"`
	Ctime      time.Time         `json:"ctime"`
	Btime      time.Time         `json:"btime"`
	Idx        uint64            `json:"Idx"`
}

func GetVFSService(config_obj *config_proto.Config) (VFSService, error) {
	org_manager, err := GetOrgManager()
	if err != nil {
		return nil, err
	}

	return org_manager.Services(config_obj.OrgId).VFSService()
}

type VFSService interface {
	// Lists all the directories in the VFS path provided. This is
	// used by the tree widget in the GUI so it only returns
	// directories. For both files and directories see ListFiles()
	// below.
	ListDirectories(
		ctx context.Context,
		config_obj *config_proto.Config,
		client_id string,
		components []string) (*api_proto.VFSListResponse, error)

	// Lists the files in the directory as well. Enriches with
	// download information for downloaed files. Used by the GUI's VFS
	// file listing widget. Supports table transformations like
	// filtering/sorting etc which can be provided with the
	// GetTableRequest.
	ListDirectoryFiles(
		ctx context.Context,
		config_obj *config_proto.Config,
		in *api_proto.GetTableRequest) (*api_proto.GetTableResponse, error)

	StatDirectory(
		config_obj *config_proto.Config,
		client_id string,
		vfs_components []string) (*api_proto.VFSListResponse, error)

	StatDownload(
		config_obj *config_proto.Config,
		client_id string,
		accessor string,
		path_components []string) (*flows_proto.VFSDownloadInfo, error)

	WriteDownloadInfo(
		ctx context.Context,
		config_obj *config_proto.Config,
		client_id string,
		accessor string,
		client_components []string,
		record *flows_proto.VFSDownloadInfo) error
}
