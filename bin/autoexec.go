package main

import (
	"os"

	"www.velocidex.com/golang/velociraptor/config"
	logging "www.velocidex.com/golang/velociraptor/logging"
)

func maybeUnpackConfig(args []string) []string {
	embedded_config, err := config.ReadEmbeddedConfig()
	if err != nil || embedded_config.Autoexec == nil {
		return args
	}

	argv := []string{}
	for _, arg := range embedded_config.Autoexec.Argv {
		argv = append(argv, os.ExpandEnv(arg))
	}

	logging.GetLogger(embedded_config, &logging.ToolComponent).
		Info("Autoexec with parameters: %s", argv)

	return argv
}
