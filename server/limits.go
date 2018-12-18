// +build !linux

package server

import (
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
)

func IncreaseLimits(config_obj *api_proto.Config) {}
