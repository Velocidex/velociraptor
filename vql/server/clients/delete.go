package clients

import (
	"context"
	"os"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/search"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type DeleteClientArgs struct {
	ClientId   string `vfilter:"required,field=client_id"`
	ReallyDoIt bool   `vfilter:"optional,field=really_do_it"`
}

type DeleteClientPlugin struct{}

func (self DeleteClientPlugin) Call(ctx context.Context,
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

		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("client_delete: %s", err)
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

		file_store_factory := file_store.GetFileStore(config_obj)
		client_path_manager := paths.NewClientPathManager(arg.ClientId)

		// Indiscriminately delete all the client's datastore files.
		err = datastore.Walk(config_obj, db, client_path_manager.Path(),
			func(filename api.DSPathSpec) error {
				select {
				case <-ctx.Done():
					return nil

				case output_chan <- ordereddict.NewDict().
					Set("client_id", arg.ClientId).
					Set("type", "Datastore").
					Set("vfs_path", filename.AsClientPath()).
					Set("really_do_it", arg.ReallyDoIt):
				}

				if arg.ReallyDoIt {
					err = db.DeleteSubject(config_obj, filename)
					if err != nil && os.IsExist(err) {
						scope.Log("client_delete: while deleting %v: %s",
							filename, err)
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
			err = reallyDeleteClient(ctx, config_obj, scope, db, arg)
			if err != nil {
				scope.Log("client_delete: %s", err)
				return
			}

		}

		// Delete the filestore files.
		err = api.Walk(file_store_factory,
			client_path_manager.Path().AsFilestorePath(),
			func(filename api.FSPathSpec, info os.FileInfo) error {
				select {
				case <-ctx.Done():
					return nil

				case output_chan <- ordereddict.NewDict().
					Set("client_id", arg.ClientId).
					Set("type", "Filestore").
					Set("vfs_path", filename.AsClientPath()).
					Set("really_do_it", arg.ReallyDoIt):
				}

				if arg.ReallyDoIt {
					err := file_store_factory.Delete(filename)
					if err != nil {
						scope.Log("client_delete: while deleting %v: %s",
							filename, err)
					}
				}
				return nil
			})
		if err != nil {
			scope.Log("client_delete: %s", err)
			return
		}

		// Notify the client to force it to disconnect in case
		// it is already up.
		notifier := services.GetNotifier()
		if notifier != nil {
			err = notifier.NotifyListener(config_obj, arg.ClientId)
			if err != nil {
				scope.Log("client_delete: %s", err)
			}
		}
	}()

	return output_chan
}

func reallyDeleteClient(ctx context.Context,
	config_obj *config_proto.Config, scope vfilter.Scope,
	db datastore.DataStore, arg *DeleteClientArgs) error {

	client_info, err := search.GetApiClient(ctx,
		config_obj, arg.ClientId, false /* detailed */)
	if err != nil {
		return err
	}

	client_path_manager := paths.NewClientPathManager(arg.ClientId)
	err = db.DeleteSubject(config_obj, client_path_manager.Path())
	if err != nil && os.IsExist(err) {
		return err
	}

	// Remove any labels
	labeler := services.GetLabeler()
	for _, label := range labeler.GetClientLabels(config_obj, arg.ClientId) {
		err := labeler.RemoveClientLabel(config_obj, arg.ClientId, label)
		if err != nil && os.IsExist(err) {
			return err
		}
	}

	// Sync up with the indexes created by the
	// interrogation service.
	keywords := []string{"all", client_info.ClientId}
	if client_info.OsInfo != nil && client_info.OsInfo.Fqdn != "" {
		keywords = append(keywords, "host:"+client_info.OsInfo.Hostname)
		keywords = append(keywords, "host:"+client_info.OsInfo.Fqdn)
	}
	for _, keyword := range keywords {
		err = search.UnsetIndex(config_obj, arg.ClientId, keyword)
		if err != nil && os.IsExist(err) {
			return err
		}
	}

	// Send an event that the client was deleted.
	journal, err := services.GetJournal()
	if err != nil {
		return err
	}

	return journal.PushRowsToArtifact(config_obj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("ClientId", arg.ClientId).
			Set("Principal", vql_subsystem.GetPrincipal(scope))},
		"Server.Internal.ClientDelete", "server", "")
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
