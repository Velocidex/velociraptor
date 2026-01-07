package main

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/Velocidex/ordereddict"
	errors "github.com/go-errors/errors"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/startup"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/vfilter"
)

var (
	verify                = artifact_command.Command("verify", "Verify a set of artifacts")
	verify_args           = verify.Arg("paths", "Paths to artifact yaml files").Required().Strings()
	verify_allow_override = verify.Flag("builtin", "Allow overriding of built in artifacts").Bool()
	verify_issues_only    = verify.Flag("issues_only", "If set, we only emit warning and error messages").Bool()
)

func doVerify() error {
	logging.DisableLogging()

	config_obj, err := makeDefaultConfigLoader().
		WithNullLoader().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to create config: %w", err)
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

	var artifact_paths []string

	for _, artifact_path := range *verify_args {
		abs, err := filepath.Abs(artifact_path)
		if err != nil {
			logger.Error("verify: could not get absolute path for %v", artifact_path)
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
			Set("Artifacts", artifact_paths).
			Set("DisableOverride", !*verify_allow_override),
	}

	query := `
		-- Load artifacts into local repository
		LET Definitions <= SELECT
			Filename,
			Data,
			artifact_set(definition=Data, repository="local") AS Definition
		FROM read_file(filenames=Artifacts)

		-- Verify artifacts from local repository
		SELECT Filename,
			Result
		FROM foreach(row=Definitions,
					query={
			SELECT Filename,
				verify(artifact=Data,
						repository="local",
						disable_override=DisableOverride) AS Result
			FROM scope()
		})
	`

	scope := manager.BuildScope(builder)
	defer scope.Close()

	statements, err := vfilter.MultiParse(query)
	if err != nil {
		logger.Error("verify: error passing query: %v", query)
		return err
	}

	var ret error
	for _, vql := range statements {
		for row := range vql.Eval(sm.Ctx, scope) {
			dict := vfilter.RowToDict(ctx, scope, row)

			artifact_path, pres := dict.GetString("Filename")
			if !pres {
				continue
			}

			result, pres := dict.Get("Result")
			if !pres {
				continue
			}

			state, ok := result.(*launcher.AnalysisState)
			if !ok {
				continue
			}
			if len(state.Errors) == 0 {
				if !*verify_issues_only {
					logger.Info("Verified %v: <green>OK</>", artifact_path)
				}
			}
			for _, err := range state.Errors {
				logger.Error("%v: <red>%v</>", artifact_path, err)
				ret = errors.New(err)
			}
			for _, msg := range state.Warnings {
				logger.Info("%v: %v", artifact_path, msg)
			}
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
