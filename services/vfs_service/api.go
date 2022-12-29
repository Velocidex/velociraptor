package vfs_service

import (
	"time"

	"github.com/Velocidex/ordereddict"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
)

// This is the type of rows sent in the
// System.VFS.ListDirectory/Listing artifact. The VFS service will
// parse them and write to the datastore.
type VFSListRow struct {
	FullPath   string                     `json:"FullPath"`
	Components []string                   `json:"Components"`
	Accessor   string                     `json:"Accessor"`
	Data       *ordereddict.Dict          `json:"Data"`
	Stats      *api_proto.VFSListResponse `json:"Stats"`
	Name       string                     `json:"Name"`
	Size       int64                      `json:"Size"`
	Mode       string                     `json:"Mode"`
	Mtime      time.Time                  `json:"mtime"`
	Atime      time.Time                  `json:"atime"`
	Ctime      time.Time                  `json:"ctime"`
	Btime      time.Time                  `json:"btime"`
}
