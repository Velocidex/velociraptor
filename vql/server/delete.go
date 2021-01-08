package server

import (
	"context"
	"os"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/api"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type DeleteClientArgs struct {
	ClientId   string `vfilter:"required,field=client_id"`
	ReallyDoIt bool   `vfilter:"optional,field=really_do_it"`
}

type DeleteClientPlugin struct{}

func (self *DeleteClientPlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {

	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &DeleteClientArgs{}

		err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
		if err != nil {
			scope.Log("client_delete: %s", err)
			return
		}

		err = vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("client_delete: %s", err.Error())
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		db, err := datastore.GetDB(config_obj)
		if err != nil {
			return
		}

		if arg.ReallyDoIt {
			client_info, err := api.GetApiClient(config_obj, nil, arg.ClientId, false)
			if err != nil {
				scope.Log("client_delete: %s", err.Error())
				return
			}

			// Remove any labels
			labeler := services.GetLabeler()
			for _, label := range labeler.GetClientLabels(config_obj, arg.ClientId) {
				err := labeler.RemoveClientLabel(config_obj, arg.ClientId, label)
				if err != nil {
					scope.Log("client_delete: %s", err.Error())
					return
				}
			}

			// Sync up with the indexes created by the interrogation service.
			keywords := []string{"all", client_info.ClientId}
			if client_info.OsInfo != nil && client_info.OsInfo.Fqdn != "" {
				keywords = append(keywords, client_info.OsInfo.Fqdn)
				keywords = append(keywords, "host:"+client_info.OsInfo.Fqdn)
			}
			err = db.UnsetIndex(config_obj, constants.CLIENT_INDEX_URN,
				arg.ClientId, keywords)
			if err != nil {
				scope.Log("client_delete: %s", err.Error())
				return
			}
		}

		file_store_factory := file_store.GetFileStore(config_obj)

		client_path_manager := paths.NewClientPathManager(arg.ClientId)

		// Indiscriminately delete all the client's datastore files.
		err = db.Walk(config_obj, client_path_manager.Path(), func(filename string) error {
			select {
			case <-ctx.Done():
				return nil

			case output_chan <- ordereddict.NewDict().
				Set("client_id", arg.ClientId).
				Set("type", "Datastore").
				Set("vfs_path", filename).
				Set("really_do_it", arg.ReallyDoIt):
			}

			if arg.ReallyDoIt {
				err = db.DeleteSubject(config_obj, filename)
				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			scope.Log("client_delete: %s", err.Error())
			return
		}

		// Delete the actual client record.
		if arg.ReallyDoIt {
			err = db.DeleteSubject(config_obj, client_path_manager.Path())
			if err != nil {
				scope.Log("client_delete: %s", err.Error())
				return
			}
		}

		// Delete the filestore files.
		err = file_store_factory.Walk(client_path_manager.Path(),
			func(filename string, info os.FileInfo, err error) error {
				select {
				case <-ctx.Done():
					return nil

				case output_chan <- ordereddict.NewDict().
					Set("client_id", arg.ClientId).
					Set("type", "Filestore").
					Set("vfs_path", filename).
					Set("really_do_it", arg.ReallyDoIt):
				}

				if arg.ReallyDoIt {
					err := file_store_factory.Delete(filename)
					if err != nil {
						scope.Log("client_delete: %s", err.Error())
					}
				}
				return nil
			})
		if err != nil {
			scope.Log("client_delete: %s", err.Error())
			return
		}

	}()

	return output_chan
}

func (self DeleteClientPlugin) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "client_delete",
		Doc:     "Delete all information related to a client. ",
		ArgType: type_map.AddType(scope, &DeleteClientArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&DeleteClientPlugin{})
}
