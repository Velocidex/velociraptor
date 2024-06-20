package notebooks

import (
	"context"
	"sync"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type CreateNotebookDownloadArgs struct {
	NotebookId string `vfilter:"required,field=notebook_id,doc=Notebook ID to export."`
	Filename   string `vfilter:"optional,field=filename,doc=The name of the export. If not set this will be named according to the notebook id and timestamp"`
}

type CreateNotebookDownload struct{}

func (self *CreateNotebookDownload) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &CreateNotebookDownloadArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("create_notebook_download: %s", err.Error())
		return vfilter.Null{}
	}

	err = vql_subsystem.CheckAccess(scope, acls.PREPARE_RESULTS)
	if err != nil {
		scope.Log("create_notebook_download: %s", err)
		return vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("create_notebook_download: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("create_notebook_download: Command can only run on the server")
		return vfilter.Null{}
	}

	wg := &sync.WaitGroup{}
	principal := vql_subsystem.GetPrincipal(scope)
	path, err := ExportNotebookToZip(ctx,
		config_obj, wg, arg.NotebookId, principal, arg.Filename)
	if err != nil {
		scope.Log("create_notebook_download: %s", err)
		return vfilter.Null{}
	}

	// Wait here until the notebook is fully exported.
	wg.Wait()

	return path
}

func (self CreateNotebookDownload) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "create_notebook_download",
		Doc:      "Creates a notebook export zip file.",
		ArgType:  type_map.AddType(scope, &CreateNotebookDownloadArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.PREPARE_RESULTS).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&CreateNotebookDownload{})
}
