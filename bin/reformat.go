package main

import (
	"fmt"
	"os"

	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/constants"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/startup"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	reformat      = artifact_command.Command("reformat", "Reformat a set of artifacts")
	reformat_args = reformat.Arg("paths", "Paths to artifact yaml files").Required().Strings()
)

func doReformat() error {
	config_obj, err := makeDefaultConfigLoader().
		WithRequiredFrontend().
		WithRequiredLogging().LoadAndValidate()
	if err != nil {
		logging.FlushPrelogs(config.GetDefaultConfig())
		return fmt.Errorf("loading config file: %w", err)
	}

	config_obj.Services = services.GenericToolServices()

	ctx, cancel := install_sig_handler()
	defer cancel()

	sm, err := startup.StartToolServices(ctx, config_obj)
	defer sm.Close()

	if err != nil {
		return err
	}

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return err
	}

	logger := logging.GetLogger(config_obj, &logging.ToolComponent)

	// Report all errors and keep going as much as possible.
	returned_errs := make(map[string]error)

	for _, artifact_path := range *reformat_args {
		returned_errs[artifact_path] = nil

		fd, err := os.Open(artifact_path)
		if err != nil {
			returned_errs[artifact_path] = err
			continue
		}

		data, err := utils.ReadAllWithLimit(fd, constants.MAX_MEMORY)
		if err != nil {
			returned_errs[artifact_path] = err
			fd.Close()
			continue
		}
		fd.Close()

		reformatted, err := manager.ReformatVQL(ctx, string(data))
		if err != nil {
			returned_errs[artifact_path] = err
			continue
		}

		out_fd, err := os.OpenFile(artifact_path,
			os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0600)
		if err != nil {
			returned_errs[artifact_path] = err
			continue
		}
		_, _ = out_fd.Write([]byte(reformatted))
		out_fd.Close()
	}

	var ret error
	for artifact_path, err := range returned_errs {
		if err != nil {
			logger.Error("%v: <red>%v</>", artifact_path, err)
			ret = err
		} else {
			logger.Info("Reformatted %v: <green>OK</>", artifact_path)
		}
	}

	return ret
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case reformat.FullCommand():
			FatalIfError(reformat, doReformat)

		default:
			return false
		}
		return true
	})
}
