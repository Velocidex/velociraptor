package main

import (
	"fmt"
	"log"

	"github.com/Velocidex/ordereddict"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/json"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/startup"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/vfilter"
)

var (
	hunts_command = app.Command(
		"hunts", "Manipulate hunts.")

	hunts_reconstruct_command = hunts_command.Command(
		"reconstruct", "Reconstruct all hunt objects from logs")
)

func doHuntReconstruct() error {
	logging.DisableLogging()

	config_obj, err := makeDefaultConfigLoader().
		WithRequiredFrontend().
		WithRequiredUser().
		LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	config_obj.Services = services.GenericToolServices()
	sm, err := startup.StartToolServices(ctx, config_obj)
	defer sm.Close()

	if err != nil {
		return err
	}

	logger := &StdoutLogWriter{}
	builder := services.ScopeBuilder{
		Config:     sm.Config,
		ACLManager: acl_managers.NewRoleACLManager(sm.Config, "administrator"),
		Logger:     log.New(logger, "", 0),
		Env:        ordereddict.NewDict(),
	}

	query := `
       SELECT * FROM source(artifact="System.Hunt.Creation")
`
	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return err
	}
	scope := manager.BuildScope(builder)
	defer scope.Close()

	statements, err := vfilter.MultiParse(query)
	if err != nil {
		return err
	}

	base_pathspec := path_specs.NewUnsafeDatastorePath("recovery", "hunts").
		SetType(api.PATH_TYPE_DATASTORE_PROTO)

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	for _, vql := range statements {
		for row := range vql.Eval(sm.Ctx, scope) {
			hunt_obj, pres := vfilter.RowToDict(ctx, scope, row).Get("Hunt")
			if !pres {
				continue
			}

			serialized := json.MustMarshalIndent(hunt_obj)
			hunt := &api_proto.Hunt{}
			err = json.Unmarshal(serialized, hunt)
			if err == nil {
				target := base_pathspec.AddChild(hunt.HuntId)
				err := db.SetSubject(config_obj, target, hunt)
				if err != nil {
					return err
				}
				fmt.Printf("Rebuilding %v to %v\n", hunt.HuntId,
					target.String())
			}
		}
	}

	return logger.Error
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case hunts_reconstruct_command.FullCommand():
			FatalIfError(hunts_reconstruct_command, doHuntReconstruct)

		default:
			return false
		}

		return true
	})
}
