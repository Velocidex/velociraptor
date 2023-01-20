package main

import (
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	timezone_flag = app.Flag(
		"timezone", "Default encoding timezone (e.g. Australia/Brisbane). If not set we use UTC").String()
)

func initTimezone(config_obj *config_proto.Config) error {
	if *timezone_flag != "" {
		return utils.SetGlobalTimezone(*timezone_flag)
	}
	return nil
}
