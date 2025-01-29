package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"www.velocidex.com/golang/velociraptor/config"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/startup"
)

var (
	verify      = app.Command("verify", "Verify a set of artifacts")
	verify_args = verify.Arg("paths", "Paths to artifact yaml files").Required().Strings()
)

func doVerify() error {
	//logging.DisableLogging()
	config_obj := config.GetDefaultConfig()

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
	var returned_err error

	repository := manager.NewRepository()
	for _, artifact_path := range *verify_args {
		fd, err := os.Open(artifact_path)
		if err != nil {
			logger.Error("%v: <red>%v</>", artifact_path, err)
			returned_err = fmt.Errorf("While processing %v: %w", artifact_path, err)
			continue
		}

		data, err := ioutil.ReadAll(fd)
		if err != nil {
			logger.Error("%v: <red>%v</>", artifact_path, err)
			returned_err = fmt.Errorf("While processing %v: %w", artifact_path, err)
			continue
		}

		_, err = repository.LoadYaml(string(data), services.ArtifactOptions{
			ValidateArtifact: true,
		})
		if err != nil {
			logger.Error("%v: <red>%v</>", artifact_path, err)
			returned_err = fmt.Errorf("While processing %v: %w", artifact_path, err)
			continue
		}
		logger.Info("Verified %v: <green>OK</>", artifact_path)
	}

	for artifact_path, a := range artifacts {
		launcher.VerifyArtifact(ctx, config_obj,
			artifact_path, a, returned_errs)
	}

	var ret error
	for artifact_path, err := range returned_errs {
		if err != nil {
			logger.Error("%v: <red>%v</>", artifact_path, err)
			ret = err
		} else {
			logger.Info("Verified %v: <green>OK</>", artifact_path)
		}
	}

	return ret
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case verify.FullCommand():
			FatalIfError(verify, doVerify)

		default:
			return false
		}
		return true
	})
}
