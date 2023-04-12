package collector

import (
	"context"
	"errors"
	"io"
	"io/ioutil"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/server/clients"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"

	_ "www.velocidex.com/golang/velociraptor/accessors/collector"
	_ "www.velocidex.com/golang/velociraptor/accessors/file"
	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

const (
	BUFF_SIZE = 1000000
)

type ImportCollectionFunctionArgs struct {
	ClientId string `vfilter:"required,field=client_id,doc=The client id to import to. Use 'auto' to generate a new client id."`
	Hostname string `vfilter:"optional,field=hostname,doc=When creating a new client, set this as the hostname."`
	Filename string `vfilter:"required,field=filename,doc=Path on server to the collector zip."`
}

type ImportCollectionFunction struct{}

func (self ImportCollectionFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.COLLECT_SERVER)
	if err != nil {
		scope.Log("import_collection: %s", err)
		return vfilter.Null{}
	}

	arg := &ImportCollectionFunctionArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("import_collection: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("import_collection: Command can only run on the server")
		return vfilter.Null{}
	}

	// Do not expand sparse files when we import them - they can be
	// deflated by the user later.
	err = vql_subsystem.CheckFilesystemAccess(scope, "collector_sparse")
	if err != nil {
		scope.Log("import_collection: %v", err)
		return vfilter.Null{}
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		scope.Log("import_collection: %v", err)
		return vfilter.Null{}
	}

	// Open the collection using the accessor
	accessor, err := accessors.GetAccessor("collector_sparse", scope)
	if err != nil {
		scope.Log("import_collection: %v", err)
		return vfilter.Null{}
	}

	root, err := accessors.NewZipFilePath("/")
	if err != nil {
		scope.Log("import_collection: %v", err)
		return vfilter.Null{}
	}

	root.SetPathSpec(&accessors.PathSpec{
		DelegateAccessor: "file",
		DelegatePath:     arg.Filename,
	})

	collection_context := &flows_proto.ArtifactCollectorContext{}
	err = self.getFile(accessor, root.Append("collection_context.json"),
		collection_context)
	if err != nil || collection_context.SessionId == "" {
		scope.Log("import_collection: unable to load collection_context: %v", err)
		return vfilter.Null{}
	}

	if arg.ClientId == "auto" || arg.ClientId == "" {
		arg.ClientId, err = self.getClientId(
			ctx, scope, config_obj, arg.Hostname)
		if err != nil {
			scope.Log("import_collection: %v", err)
			return vfilter.Null{}
		}
	}

	flow_path_manager := paths.NewFlowPathManager(
		arg.ClientId, collection_context.SessionId)

	collection_context.ClientId = arg.ClientId

	err = db.SetSubject(config_obj, flow_path_manager.Path(), collection_context)
	if err != nil {
		scope.Log("import_collection: %v", err)
		return vfilter.Null{}
	}

	// Copy the requests for provenance
	tasks := &api_proto.ApiFlowRequestDetails{}
	err = self.getFile(accessor, root.Append("requests.json"), tasks)
	if err == nil {
		err = db.SetSubject(config_obj, flow_path_manager.Task(), tasks)
		if err != nil {
			scope.Log("import_collection: %v", err)
			return vfilter.Null{}
		}
	} else {
		scope.Log("import_collection: %v", err)
	}

	// Copy the logs results set over.
	err = self.copyResultSet(ctx, config_obj, scope,
		accessor, root.Append("log.json"), flow_path_manager.Log())
	if err != nil {
		scope.Log("import_collection: %v", err)
		return vfilter.Null{}
	}

	// Now Copy the results.
	for _, artifact := range collection_context.ArtifactsWithResults {
		artifact_path_manager := artifacts.NewArtifactPathManagerWithMode(
			config_obj, arg.ClientId, collection_context.SessionId,
			artifact, paths.MODE_CLIENT)
		err = self.copyResultSet(ctx, config_obj, scope,
			accessor, root.Append("results", artifact+".json"),
			artifact_path_manager.Path())
		if err != nil {
			scope.Log("import_collection: %v", err)
		}
	}

	// Now copy any uploads - first get the metadata.
	err = self.copyResultSet(ctx, config_obj, scope,
		accessor, root.Append("uploads.json"),
		flow_path_manager.UploadMetadata())
	if err == nil {
		// It is not an error if there is no uploads metadata - it
		// just means that there were no uploads.

		// Open the upload metadata and try to find the actual files in
		// the container.
		file_store_factory := file_store.GetFileStore(config_obj)
		reader, err := result_sets.NewResultSetReader(file_store_factory,
			flow_path_manager.UploadMetadata())
		if err != nil {
			scope.Log("import_collection: %v", err)
			return vfilter.Null{}
		}
		defer reader.Close()

		for row := range reader.Rows(ctx) {
			// Do not copy index files specifically - the index file
			// will be copied as part of the file it belongs to.
			row_type, _ := row.GetString("Type")
			if row_type == "idx" {
				continue
			}

			components, pres := row.GetStrings("_Components")
			if !pres || len(components) < 1 {
				continue
			}

			// Copy from the archive to the file store at these locations.
			src := root.Append(components...)

			// First directory in zip file is "upload" we skip that
			// and append the other components to the filestore path.
			dest := flow_path_manager.UploadContainer().AddChild(components[1:]...)

			err := self.copyFileWithIndex(ctx, config_obj, scope,
				accessor, src, dest)
			if err != nil {
				scope.Log("import_collection: %v", err)
			}
		}
	}

	return collection_context
}

func (self ImportCollectionFunction) getClientId(
	ctx context.Context, scope types.Scope,
	config_obj *config_proto.Config, hostname string) (string, error) {

	if hostname != "" {
		indexer, err := services.GetIndexer(config_obj)
		if err != nil {
			return "", err
		}

		scope.Log("Searching for a client id with hostname '%v'", hostname)

		// Search for an existing client with the same hostname
		search_resp, err := indexer.SearchClients(ctx, config_obj,
			&api_proto.SearchClientsRequest{Query: "host:" + hostname}, "")
		if err == nil {
			for _, resp := range search_resp.Items {
				if strings.EqualFold(resp.OsInfo.Hostname, hostname) {
					scope.Log("client id found '%v'", resp.ClientId)
					return resp.ClientId, nil
				}
			}
		}
	}

	// Create a new client
	res := clients.NewClientFunction{}.Call(ctx, scope, ordereddict.NewDict().
		Set("first_seen_at", time.Now()).
		Set("last_seen_at", time.Now()).
		Set("hostname", hostname))
	if !utils.IsNil(res) {
		client_id_any, pres := scope.Associative(res, "ClientId")
		if pres {
			client_id, ok := client_id_any.(string)
			if ok {
				scope.Log("Creating a new client with id '%v'", client_id)
				return client_id, nil
			}
		}
	}

	client_id := clients.NewClientId()
	scope.Log("Creating a new client id '%v'", client_id)
	return client_id, nil
}

func (self ImportCollectionFunction) copyResultSet(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	accessor accessors.FileSystemAccessor,
	src *accessors.OSPath, dest api.FSPathSpec) error {

	err := self.copyFile(ctx, config_obj, scope,
		accessor, src, dest)
	if err != nil {
		return err
	}

	return self.copyFile(ctx, config_obj, scope,
		accessor, src.Dirname().Append(src.Basename()+".index"),
		dest.SetType(api.PATH_TYPE_FILESTORE_JSON_INDEX))
}

func (self ImportCollectionFunction) getFile(
	accessor accessors.FileSystemAccessor,
	path *accessors.OSPath, target interface{}) error {

	fd, err := accessor.OpenWithOSPath(path)
	if err != nil {
		return err
	}
	defer fd.Close()

	limitedReader := &io.LimitedReader{R: fd, N: BUFF_SIZE}
	data, err := ioutil.ReadAll(limitedReader)
	if err != nil && err != io.EOF {
		return err
	}

	return json.Unmarshal(data, target)
}

func (self ImportCollectionFunction) copyFileWithIndex(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	accessor accessors.FileSystemAccessor,
	src *accessors.OSPath, dest api.FSPathSpec) error {
	err := self.copyFile(
		ctx, config_obj, scope, accessor, src, dest)
	if err != nil {
		return err
	}

	err = self.copyFile(
		ctx, config_obj, scope,
		accessor, src.Dirname().Append(src.Basename()+".idx"),
		dest.SetType(api.PATH_TYPE_FILESTORE_SPARSE_IDX))
	if err != nil {
		// No idx file - not an error just means this file is not
		// sparse.
		return nil
	}
	return nil
}

func (self ImportCollectionFunction) copyFile(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	accessor accessors.FileSystemAccessor,
	src *accessors.OSPath, dest api.FSPathSpec) error {

	fd, err := accessor.OpenWithOSPath(src)
	if err != nil {
		return err
	}
	defer fd.Close()

	file_store_factory := file_store.GetFileStore(config_obj)
	out_fd, err := file_store_factory.WriteFile(dest)
	if err != nil {
		return err
	}
	defer out_fd.Close()

	out_fd.Truncate()

	scope.Log("import_collection: Copying %v to %v", src.String(), dest.AsClientPath())

	_, err = utils.Copy(ctx, out_fd, fd)
	if err != nil {
		scope.Log("import_collection: Error copying %v", err)
	}

	return err
}

func (self ImportCollectionFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "import_collection",
		Doc:      "Imports an offline collection zip file (experimental).",
		ArgType:  type_map.AddType(scope, &ImportCollectionFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.COLLECT_SERVER, acls.FILESYSTEM_READ).Build(),
	}
}

func getExistingClientOrNewClient(
	ctx context.Context,
	scope vfilter.Scope,
	config_obj *config_proto.Config,
	hostname string) (string, error) {

	indexer, err := services.GetIndexer(config_obj)
	if err != nil {
		return "", err
	}

	scope.Log("Searching for a client id with name '%v'", hostname)

	// Search for an existing client with the same hostname
	search_resp, err := indexer.SearchClients(ctx, config_obj,
		&api_proto.SearchClientsRequest{Query: "host:" + hostname}, "")
	if err == nil && len(search_resp.Items) > 0 {
		client_id := search_resp.Items[0].ClientId
		scope.Log("client id found '%v'", client_id)
		return client_id, nil
	}

	return makeNewClient(config_obj, scope, hostname)
}

// Create a new client record
func makeNewClient(
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	hostname string) (string, error) {

	if hostname == "" {
		return "", errors.New("New clients must have a hostname")
	}

	client_id := clients.NewClientId()
	client_info := &actions_proto.ClientInfo{
		ClientId:     client_id,
		Hostname:     hostname,
		Fqdn:         hostname,
		Architecture: "Offline",
		ClientName:   "OfflineVelociraptor",
	}

	scope.Log("Creating new client '%v'", client_id)

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return "", err
	}

	indexer, err := services.GetIndexer(config_obj)
	if err != nil {
		return "", err
	}

	client_path_manager := paths.NewClientPathManager(client_id)
	err = db.SetSubject(config_obj,
		client_path_manager.Path(), client_info)
	if err != nil {
		return "", err
	}

	// Add the new client to the index.
	for _, term := range []string{
		"all", // This is used for "." search
		client_id,
		"host:" + client_info.Fqdn,
		"host:" + client_info.Hostname,
	} {
		err = indexer.SetIndex(client_id, term)
		if err != nil {
			return client_id, err
		}
	}

	return client_id, nil
}

func init() {
	vql_subsystem.RegisterFunction(&ImportCollectionFunction{})
}
