//go:build !windows
// +build !windows

package writeback

import (
	"strings"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func GetFileWritebackStore(config_obj *config_proto.Config) WritebackStorer {
	location, _ := WritebackLocation(config_obj)

	// The pool client uses in-memory writebacks so no files are
	// written to disk.
	if strings.HasPrefix(location, "memory://") {
		return &MemoryWritebackStore{}
	}

	return &FileWritebackStore{
		config_obj:  config_obj,
		location:    location,
		l2_location: location + config_obj.Client.Level2WritebackSuffix,
	}
}
