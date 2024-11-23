package main

import (
	"io/ioutil"
	"os"

	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/config"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/startup"
)

var (
	verify                = app.Command("verify", "Verify a set of artifacts")
	verify_args           = verify.Arg("paths", "Paths to artifact yaml files").Required().Strings()
	verify_allow_override = verify.Flag("builtin", "Allow overriding of built in artifacts").Bool()
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
	returned_errs := make(map[string]error)

	artifacts := make(map[string]*artifacts_proto.Artifact)

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}
	for _, artifact_path := range *verify_args {
		returned_errs[artifact_path] = nil

		fd, err := os.Open(artifact_path)
		if err != nil {
			returned_errs[artifact_path] = err
			continue
		}

		data, err := ioutil.ReadAll(fd)
		if err != nil {
			returned_errs[artifact_path] = err
			continue
		}

		a, err := repository.LoadYaml(string(data), services.ArtifactOptions{
			ValidateArtifact:     true,
			ArtifactIsBuiltIn:    *verify_allow_override,
			AllowOverridingAlias: true,
		})
		if err != nil {
			returned_errs[artifact_path] = err
			continue
		}
		artifacts[artifact_path] = a
	}

	for artifact_path, a := range artifacts {
		if a.Precondition != "" {
			for _, err := range launcher.VerifyVQL(ctx, config_obj,
				a.Precondition, repository) {
				returned_errs[artifact_path] = err
			}
		}

		for _, s := range a.Sources {
			if s.Query != "" {
				dependency := make(map[string]int)

				err := launcher.GetQueryDependencies(ctx, config_obj,
					repository, s.Query, 0, dependency)
				if err != nil {
					returned_errs[artifact_path] = err
					continue
				}

				// Now check for broken callsites
				for _, err := range launcher.VerifyVQL(ctx, config_obj,
					s.Query, repository) {
					returned_errs[artifact_path] = err
				}
			}
			if s.Precondition != "" {
				for _, err := range launcher.VerifyVQL(ctx, config_obj,
					s.Precondition, repository) {
					returned_errs[artifact_path] = err
				}
			}
		}
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
