package clients

import (
	"context"
	"errors"
	"os"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
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

		if !constants.ClientIdRegex.MatchString(arg.ClientId) {
			scope.Log("ERROR:client_delete: Client Id should be of the form C.XXXX")
			return
		}

		err = services.RequireFrontend()
		if err != nil {
			scope.Log("client_delete: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("client_delete: Command can only run on the server")
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
			datastore.WalkWithoutDirectories,
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
					err := db.DeleteSubject(config_obj, filename)
					if err != nil && errors.Is(err, os.ErrNotExist) {
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

		// Remove the empty directories
		err = datastore.Walk(config_obj, db, client_path_manager.Path(),
			datastore.WalkWithDirectories,
			func(filename api.DSPathSpec) error {
				err := db.DeleteSubject(config_obj, filename)
				if err != nil {
					scope.Log("client_delete: Removing directory %v: %v",
						filename.AsClientPath(), err)
				}
				return nil
			})

		// Delete the actual client record.
		if arg.ReallyDoIt {
			err = reallyDeleteClient(ctx, config_obj, scope, db, arg)
			if err != nil {
				scope.Log("client_delete: %s", err)
				return
			}

			// Finally remove the containing directory
			err = db.DeleteSubject(
				config_obj,
				paths.NewClientPathManager(arg.ClientId).Path().SetDir())
			if err != nil {
				scope.Log("client_delete: %s", err)
			}
		}

		// Notify the client to force it to disconnect in case
		// it is already up.
		notifier, err := services.GetNotifier(config_obj)
		if err == nil {
			err = notifier.NotifyListener(ctx,
				config_obj, arg.ClientId, "DeleteClient")
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

	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return err
	}

	client_info_manager.Remove(ctx, arg.ClientId)

	indexer, err := services.GetIndexer(config_obj)
	if err != nil {
		return err
	}

	client_info, err := indexer.FastGetApiClient(ctx,
		config_obj, arg.ClientId)
	if err != nil {
		return err
	}

	client_path_manager := paths.NewClientPathManager(arg.ClientId)
	err = db.DeleteSubject(config_obj, client_path_manager.Path())
	if err != nil && errors.Is(err, os.ErrNotExist) {
		return err
	}

	// Remove any labels
	labeler := services.GetLabeler(config_obj)
	for _, label := range labeler.GetClientLabels(ctx, config_obj, arg.ClientId) {
		err := labeler.RemoveClientLabel(ctx, config_obj, arg.ClientId, label)
		if err != nil && errors.Is(err, os.ErrNotExist) {
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
		err = indexer.UnsetIndex(arg.ClientId, keyword)
		if err != nil && errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return err
	}

	principal := vql_subsystem.GetPrincipal(scope)
	services.LogAudit(ctx,
		config_obj, principal, "client_delete",
		ordereddict.NewDict().
			Set("client_id", arg.ClientId).
			Set("org_id", config_obj.OrgId))

	err = journal.PushRowsToArtifact(ctx, config_obj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("ClientId", arg.ClientId).
			Set("OrgId", config_obj.OrgId).
			Set("Principal", principal)},
		"Server.Internal.ClientDelete", "server", "")

	if err != nil {
		return err
	}

	// Send an event that the client was deleted to the root org as
	// well. The Frontend is not org aware and needs to be informed to
	// client deletion events.
	if utils.IsRootOrg(config_obj.OrgId) {
		return nil
	}

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return err
	}

	root_config_obj, err := org_manager.GetOrgConfig("root")
	if err != nil {
		return err
	}

	journal, err = services.GetJournal(root_config_obj)
	if err != nil {
		return err
	}

	return journal.PushRowsToArtifact(ctx, root_config_obj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("ClientId", arg.ClientId).
			Set("OrgId", config_obj.OrgId).
			Set("Principal", principal)},
		"Server.Internal.ClientDelete", "server", "")
}

func (self DeleteClientPlugin) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "client_delete",
		Doc:      "Delete all information related to a client. ",
		ArgType:  type_map.AddType(scope, &DeleteClientArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.DELETE_RESULTS).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&DeleteClientPlugin{})
}
