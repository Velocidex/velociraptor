// +build !windows

package writeback

import config_proto "www.velocidex.com/golang/velociraptor/config/proto"

func GetFileWritebackStore(config_obj *config_proto.Config) WritebackStorer {
	location, _ := WritebackLocation(config_obj)

	return &FileWritebackStore{
		config_obj:  config_obj,
		location:    location,
		l2_location: location + config_obj.Client.Level2WritebackSuffix,
	}
}
