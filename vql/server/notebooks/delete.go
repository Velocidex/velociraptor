package notebooks

import (
	"context"
	"os"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/reporting"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type DeleteNotebookArgs struct {
	NotebookId string `vfilter:"required,field=notebook_id"`
	ReallyDoIt bool   `vfilter:"optional,field=really_do_it"`
}

type DeleteNotebookPlugin struct{}

func (self *DeleteNotebookPlugin) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {

	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &DeleteNotebookArgs{}

		err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
		if err != nil {
			scope.Log("notebook_delete: %s", err)
			return
		}

		err = vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("notebook_delete: %s", err.Error())
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

		notebook_path_manager := reporting.NewNotebookPathManager(arg.NotebookId)

		if arg.ReallyDoIt {
			err = db.DeleteSubject(config_obj, notebook_path_manager.Path())
			if err != nil {
				scope.Log("notebook_delete: %s", err.Error())
				return
			}
		}

		// Indiscriminately delete all the client's datastore files.
		err = db.Walk(config_obj, notebook_path_manager.Directory(),
			func(filename string) error {
				select {
				case <-ctx.Done():
					return nil

				case output_chan <- ordereddict.NewDict().
					Set("notebook_id", arg.NotebookId).
					Set("type", "Notebook").
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
			scope.Log("notebook_delete: %s", err.Error())
			return
		}

		// Delete the filestore files.
		err = file_store_factory.Walk(notebook_path_manager.Directory(),
			func(filename string, info os.FileInfo, err error) error {
				select {
				case <-ctx.Done():
					return nil

				case output_chan <- ordereddict.NewDict().
					Set("notebook_id", arg.NotebookId).
					Set("type", "Filestore").
					Set("vfs_path", filename).
					Set("really_do_it", arg.ReallyDoIt):
				}

				if arg.ReallyDoIt {
					err := file_store_factory.Delete(filename)
					if err != nil {
						scope.Log("notebook_delete: %s", err.Error())
					}
				}
				return nil
			})
		if err != nil {
			scope.Log("notebook_delete: %s", err.Error())
			return
		}

	}()

	return output_chan
}

func (self DeleteNotebookPlugin) Info(
	scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "notebook_delete",
		Doc:     "Delete a notebook with all its cells. ",
		ArgType: type_map.AddType(scope, &DeleteNotebookArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&DeleteNotebookPlugin{})
}
