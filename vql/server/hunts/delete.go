package hunts

import (
	"context"
	"errors"
	"os"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type DeleteHuntArgs struct {
	HuntId     string `vfilter:"required,field=hunt_id"`
	ReallyDoIt bool   `vfilter:"optional,field=really_do_it"`
}

type DeleteHuntPlugin struct{}

func (self DeleteHuntPlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {

	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &DeleteHuntArgs{}

		err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
		if err != nil {
			scope.Log("hunt_delete: %s", err)
			return
		}

		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("hunt_delete: %s", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		// Now remove the hunt from the hunt manager
		if arg.ReallyDoIt {
			mutation := api_proto.HuntMutation{
				HuntId: arg.HuntId,
				State:  api_proto.Hunt_ARCHIVED,
			}
			journal, err := services.GetJournal()
			if err != nil {
				scope.Log("hunt_delete: %s", err)
				return
			}

			journal.PushRowsToArtifactAsync(config_obj,
				ordereddict.NewDict().
					Set("hunt_id", arg.HuntId).
					Set("mutation", mutation),
				"Server.Internal.HuntModification")
		}

		db, err := datastore.GetDB(config_obj)
		if err != nil {
			return
		}

		file_store_factory := file_store.GetFileStore(config_obj)
		hunt_path_manager := paths.NewHuntPathManager(arg.HuntId)

		// Indiscriminately delete all the hunts's datastore files.
		err = datastore.Walk(config_obj, db, hunt_path_manager.Path(),
			func(filename api.DSPathSpec) error {
				select {
				case <-ctx.Done():
					return nil

				case output_chan <- ordereddict.NewDict().
					Set("hunt_id", arg.HuntId).
					Set("type", "Datastore").
					Set("vfs_path", filename.AsClientPath()).
					Set("really_do_it", arg.ReallyDoIt):
				}

				if arg.ReallyDoIt {
					err = db.DeleteSubject(config_obj, filename)
					if err != nil && errors.Is(err, os.ErrNotExist) {
						scope.Log("hunt_delete: while deleting %v: %s",
							filename, err)
					}
				}
				return nil
			})
		if err != nil {
			scope.Log("hunt_delete: %s", err.Error())
			return
		}

		// Delete the filestore files.
		err = api.Walk(file_store_factory,
			hunt_path_manager.Path().AsFilestorePath(),
			func(filename api.FSPathSpec, info os.FileInfo) error {
				select {
				case <-ctx.Done():
					return nil

				case output_chan <- ordereddict.NewDict().
					Set("hunt_id", arg.HuntId).
					Set("type", "Filestore").
					Set("vfs_path", filename.AsClientPath()).
					Set("really_do_it", arg.ReallyDoIt):
				}

				if arg.ReallyDoIt {
					err := file_store_factory.Delete(filename)
					if err != nil {
						scope.Log("hunt_delete: while deleting %v: %s",
							filename, err)
					}
				}
				return nil
			})
		if err != nil {
			scope.Log("hunt_delete: %s", err)
			return
		}

	}()

	return output_chan
}

func (self DeleteHuntPlugin) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "hunt_delete",
		Doc:     "Delete a hunt. ",
		ArgType: type_map.AddType(scope, &DeleteHuntArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&DeleteHuntPlugin{})
}
