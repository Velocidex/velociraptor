package main

import (
	"os"

	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func maybeUnpackConfig(args []string) ([]string, *config_proto.Config) {
	embedded_config, err := config.ReadEmbeddedConfig()
	if err != nil || embedded_config.Autoexec == nil {
		return args, nil
	}

	argv := []string{}
	for _, arg := range embedded_config.Autoexec.Argv {
		argv = append(argv, os.ExpandEnv(arg))
	}

	return argv, embedded_config
}
