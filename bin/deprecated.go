package main

import (
	errors "github.com/go-errors/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func deprecatedOverride(config_obj *config_proto.Config) error {
	if *override_flag != "" {
		return errors.New("The --config_override flag is deprecated. Please use one of --merge, --merge_path, --patch, --patch_path instead")
	}
	return nil
}
