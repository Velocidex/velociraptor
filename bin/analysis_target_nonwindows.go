//go:build !windows
// +build !windows

package main

import (
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

func applyAnalysisTarget(config_obj *config_proto.Config) error {
	if config_obj.AnalysisTarget == "" {
		return nil
	}

	switch config_obj.AnalysisTarget {
	case "windows":
		vql_subsystem.MakeNoopPlugin("wmi")
		break

	default:
		break
	}

	return nil
}
