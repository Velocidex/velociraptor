package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/config"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/startup"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	"www.velocidex.com/golang/vfilter"
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
	var artifact_paths []string

	for _, artifact_path := range *reformat_args {
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
			Set("Artifacts", artifact_paths),
	}

	query := `
		SELECT FilePath,
			   Result
		FROM foreach(row=Artifacts,
					 query={
			SELECT _value AS FilePath,
				   reformat(artifact=read_file(filename=_value)) AS Result
			FROM scope()
		})
	`

	scope := manager.BuildScope(builder)
	defer scope.Close()

	statements, err := vfilter.MultiParse(query)
	if err != nil {
		logger.Error("reformat: error parsing query: %v", query)
		return err
	}

	for _, vql := range statements {
		for row := range vql.Eval(sm.Ctx, scope) {
			dict := vfilter.RowToDict(ctx, scope, row)

			artifact_path, pres := dict.GetString("FilePath")
			if !pres {
				continue
			}

			result_val, pres := dict.Get("Result")
			if !pres {
				continue
			}

			result, ok := result_val.(*functions.ReformatFunctionResult)
			if !ok {
				continue
			}

			if result.Error != nil {
				returned_errs[artifact_path] = result.Error
				continue
			}

			out_fd, err := os.OpenFile(artifact_path, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0600)
			if err != nil {
				returned_errs[artifact_path] = err
				continue
			}
			_, _ = out_fd.Write([]byte(result.Artifact))
			out_fd.Close()

			returned_errs[artifact_path] = nil
		}
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
