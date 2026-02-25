package main

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/config"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/startup"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
)

var (
	reformat         = artifact_command.Command("reformat", "Reformat a set of artifacts")
	reformat_dry_run = reformat.Flag("dry", "Do not overwrite files, just report errors").Bool()
	reformat_args    = reformat.Arg("paths", "Paths to artifact yaml files").Required().Strings()
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

	logger := logging.GetLogger(config_obj, &logging.ToolComponent)

	var artifact_paths []string
	for _, artifact_path := range expandGlobs(*reformat_args) {
		abs, err := filepath.Abs(artifact_path)
		if err != nil {
			logger.Error("reformat: could not get absolute path for %v", artifact_path)
			continue
		}

		artifact_paths = append(artifact_paths, abs)
	}

	artifact_logger := &LogWriter{config_obj: sm.Config}
	builder := services.ScopeBuilder{
		Config:     sm.Config,
		ACLManager: acl_managers.NewRoleACLManager(sm.Config, "administrator"),
		Logger:     log.New(artifact_logger, "", 0),
		Env: ordereddict.NewDict().
			Set("DryRun", *reformat_dry_run).
			Set("Artifacts", artifact_paths),
	}

	query := `
      SELECT reformat(artifact=read_file(filename=_value)) AS Result
        FROM foreach(row=Artifacts)
      WHERE if(condition=Result.Error,
          then=log(level="ERROR", message="%v: <red>%v</>",
                   args=[_value, Result.Error], dedup=-1),
          else=log(message="Reformatted %v: <green>OK</>",
                   args=_value, dedup=-1)
               AND NOT DryRun
               AND copy(accessor="data", dest=_value, filename=Result.Artifact))
       AND FALSE
    `
	err = runQueryWithEnv(query, builder, "json")
	if err != nil {
		logger.Error("reformat: error running query: %v", query)
	}

	return artifact_logger.Error
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
