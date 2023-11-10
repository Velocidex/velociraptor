package collector

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"io/ioutil"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
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
	"www.velocidex.com/golang/velociraptor/services/hunt_dispatcher"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/server/clients"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"

	_ "www.velocidex.com/golang/velociraptor/accessors/collector"
	_ "www.velocidex.com/golang/velociraptor/accessors/file_store"
	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

const (
	BUFF_SIZE = 1000000
)

type ImportCollectionFunctionArgs struct {
	ClientId   string `vfilter:"optional,field=client_id,doc=The client id to import to. Use 'auto' to generate a new client id."`
	Hostname   string `vfilter:"optional,field=hostname,doc=When creating a new client, set this as the hostname."`
	Filename   string `vfilter:"required,field=filename,doc=Path on server to the collector zip."`
	Accessor   string `vfilter:"optional,field=accessor,doc=The accessor to use."`
	ImportType string `vfilter:"optional,field=import_type,doc=Whether the import is an offline_collector or hunt."`
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
		DelegateAccessor: arg.Accessor,
		DelegatePath:     arg.Filename,
	})

	if arg.ImportType != "hunt" && arg.ImportType != "collector" {
		arg.ImportType = ""
	}

	if arg.ImportType == "" {
		_, err = self.checkHuntInfo(root, accessor)
		if err != nil {
			arg.ImportType = "collector"
		} else {
			arg.ImportType = "hunt"
		}
	}

	if arg.ImportType == "collector" {
		return self.importFlow(
			ctx, scope, config_obj,
			db, accessor, root, arg.ClientId,
			arg.Hostname, false)

	} else if arg.ImportType == "hunt" {
		// Check if there is a hunt_info.json. This won't work with
		// older exports (<0.7.1) because we previously didn't export
		// all the hunt information.
		hunt_info, err := self.checkHuntInfo(root, accessor)
		if err != nil {
			scope.Log("import_collection: %v", err)
			return vfilter.Null{}
		}

		// Update the huntId in case it was already taken.
		hunt_info.HuntId, err = self.importHunt(ctx, scope, config_obj, hunt_info)
		if err != nil {
			scope.Log("import_collection: %v", err)
			return vfilter.Null{}
		}

		directory_listing, err := accessor.ReadDirWithOSPath(root)
		if err != nil {
			scope.Log("import_collection: %v", err)
			return vfilter.Null{}
		}

		for _, item := range directory_listing {
			if !item.IsDir() || item.Name() == "results" || item.Name() == "uploads" {
				continue
			}

			path := root.Append(item.Name())

			client_info := &services.ClientInfo{}
			err = self.getFile(accessor, path.Append("client_info.json"), client_info)
			if err != nil {
				continue
			}

			err = self.checkClientIdExists(ctx, config_obj, scope, client_info)
			if err != nil {
				continue
			}

			self.importFlow(
				ctx, scope, config_obj, db,
				accessor, path, client_info.ClientId, client_info.Hostname, true)

		}

		return hunt_info
	} else {
		return vfilter.Null{}
	}
}

func (self ImportCollectionFunction) importFlow(
	ctx context.Context,
	scope vfilter.Scope,
	config_obj *config_proto.Config,
	db datastore.DataStore,
	accessor accessors.FileSystemAccessor,
	root *accessors.OSPath,
	client_id string,
	hostname string,
	hunt bool) vfilter.Any {

	collection_context := &flows_proto.ArtifactCollectorContext{}
	path := root.Append("collection_context.json")

	err := self.getFile(accessor, path, collection_context)
	if err != nil || collection_context.SessionId == "" {
		// Support older collections which were encoded a bit
		// differently.
		details := &api_proto.FlowDetails{}
		err := self.getFile(accessor, path, details)
		if err != nil {
			scope.Log("import_flow: unable to load collection_context: %v", err)
			return vfilter.Null{}
		}
		collection_context = details.Context
	}

	if client_id == "auto" || client_id == "" {
		client_id, err = self.getClientIdFromHostname(
			ctx, scope, config_obj, hostname)
		if err != nil {
			scope.Log("import_flow: %v", err)
			return vfilter.Null{}
		}
	}

	collection_context.ClientId = client_id

	flow_path_manager := paths.NewFlowPathManager(
		client_id, collection_context.SessionId)

	err = db.SetSubject(config_obj, flow_path_manager.Path(), collection_context)
	if err != nil {
		scope.Log("import_flow: %v", err)
		return vfilter.Null{}
	}

	// Copy the requests for provenance
	tasks := &api_proto.ApiFlowRequestDetails{}
	err = self.getFile(accessor, root.Append("requests.json"), tasks)
	if err == nil {
		err = db.SetSubject(config_obj, flow_path_manager.Task(), tasks)
		if err != nil {
			scope.Log("import_flow: %v", err)
			return vfilter.Null{}
		}
	} else {
		scope.Log("import_flow: %v", err)
	}

	// Copy the logs results set over.
	err = self.copyResultSet(ctx, config_obj, scope,
		accessor, root.Append("log.json"), flow_path_manager.Log())
	if err != nil {
		err = self.copyResultSet(ctx, config_obj, scope,
			accessor, root.Append("logs.json"), flow_path_manager.Log())
		if err != nil {
			scope.Log("import_flow: %v", err)
		}
	}

	// Now Copy the results.
	for _, artifact := range collection_context.ArtifactsWithResults {
		artifact_path_manager := artifacts.NewArtifactPathManagerWithMode(
			config_obj, client_id, collection_context.SessionId,
			artifact, paths.MODE_CLIENT)
		err = self.copyResultSet(ctx, config_obj, scope,
			accessor, root.Append("results", artifact+".json"),
			artifact_path_manager.Path())
		if err != nil {
			scope.Log("import_flow: %v", err)
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
			scope.Log("import_flow: %v", err)
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
				scope.Log("import_flow: %v", err)
			}
		}
	}

	err = self.copyFile(ctx, config_obj, scope, accessor, root.Append("Notebook.yaml"),
		flow_path_manager.Path().AsFilestorePath().AddChild("Notebook.yaml"))
	if err == nil {
		// It is not an error if there is no uploads metadata - it
		// just means that there were no uploads.

		// Open the upload metadata and try to find the actual files in
		// the container.
		err = self.CopyDirectory(ctx, config_obj, scope, accessor,
			root.Append("notebooks"), flow_path_manager.Path().AsFilestorePath().AddChild("notebook"))
		if err != nil {
			scope.Log("import_flow: %v", err)
			return vfilter.Null{}
		}
	}

	// If we got here - all went well and we can emit an event to let
	// listeners know there is a new collection.
	row := ordereddict.NewDict().
		Set("Timestamp", utils.GetTime().Now().UTC().Unix()).
		Set("Flow", collection_context).
		Set("FlowId", collection_context.SessionId).
		Set("ClientId", collection_context.ClientId)

	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return collection_context
	}
	journal.PushRowsToArtifact(ctx, config_obj,
		[]*ordereddict.Dict{row},
		"System.Flow.Completion", collection_context.ClientId,
		collection_context.SessionId,
	)

	return collection_context
}

func (self ImportCollectionFunction) importHunt(
	ctx context.Context,
	scope vfilter.Scope,
	config_obj *config_proto.Config,
	hunt *api_proto.Hunt) (string, error) {

	hunt_disp, err := services.GetHuntDispatcher(config_obj)
	if err != nil {
		return "", err
	}

	// Check if valid hunt id and no duplicates.
	if hunt.HuntId == "" {
		hunt.HuntId = hunt_dispatcher.GetNewHuntId()
	} else {
		// If it has a hunt id, see if it already exists,
		// create a new one if so.
		_, pres := hunt_disp.GetHunt(hunt.HuntId)
		if pres {
			hunt.HuntId = hunt_dispatcher.GetNewHuntId()
		}
	}

	hunt.State = api_proto.Hunt_STOPPED

	manager_any, pres := scope.Resolve(vql_subsystem.ACL_MANAGER_VAR)
	if !pres {
		return "", errors.New("No ACL manager")
	}

	acl_manager, ok := manager_any.(vql_subsystem.ACLManager)
	if !ok {
		return "", errors.New("No ACL manager")
	}

	_, err = hunt_disp.CreateHunt(ctx, config_obj, acl_manager, hunt)
	return hunt.HuntId, err
}

func (self ImportCollectionFunction) getClientIdFromHostname(
	ctx context.Context,
	scope types.Scope,
	config_obj *config_proto.Config,
	hostname string) (string, error) {

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

func (self ImportCollectionFunction) checkClientIdExists(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	client_info *services.ClientInfo) error {

	// This is a well known client
	if client_info.ClientId == "server" {
		return nil
	}

	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return err
	}

	_, err = client_info_manager.Get(ctx, client_info.ClientId)
	if err == nil {
		return nil
	}

	scope.Log("Info: ClientId %v (%v) not found, creating a new client",
		client_info.ClientId, client_info.Hostname)

	// If we made it here, client id doesn't exist, so then we can
	// create a new client
	res := clients.NewClientFunction{}.Call(ctx, scope, ordereddict.NewDict().
		Set("client_id", client_info.ClientId).
		Set("first_seen_at", utils.GetTime().Now()).
		Set("last_seen_at", utils.GetTime().Now()).
		Set("hostname", client_info.Hostname).
		Set("labels", client_info.Labels).
		Set("os", client_info.System).
		Set("mac_addresses", client_info.MacAddresses))

	if res == nil {
		return errors.New("Failed to create new client.")
	}

	return nil
}

func (self ImportCollectionFunction) copyResultSet(
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

	// Read the JSONL from the zip and write into a new result set. We
	// could just copy the JSONL across but there are non disk based
	// filestores so we need to do it the slow way.
	file_store_factory := file_store.GetFileStore(config_obj)
	rs_writer, err := result_sets.NewResultSetWriter(file_store_factory,
		dest, json.DefaultEncOpts(), utils.SyncCompleter, result_sets.TruncateMode)
	if err != nil {
		return err
	}
	defer rs_writer.Close()

	// Copy the file into the results set 100 rows at the time.
	reader := bufio.NewReader(fd)
	count := uint64(0)
	buffer := bytes.Buffer{}

	flush := func() {
		if count > 0 {
			rs_writer.WriteJSONL(buffer.Bytes(), count)
			count = 0
			buffer.Reset()
		}
	}
	defer flush()

	for {
		select {
		case <-ctx.Done():
			return nil

		default:
			row_data, err := reader.ReadBytes('\n')
			if err != nil {
				return nil
			}
			buffer.Write(row_data)
			count++

			// Dump chunks into the result set - this is much faster
			// than parsing the encoding.
			if count > 100 {
				flush()
			}
		}
	}

	return nil
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

	scope.Log("import_collection: Copying %v to %v", src.String(), dest.String())

	_, err = utils.Copy(ctx, out_fd, fd)
	if err != nil {
		scope.Log("import_collection: Error copying %v", err)
	}

	return err
}

func (self ImportCollectionFunction) CopyDirectory(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	accessor accessors.FileSystemAccessor,
	src *accessors.OSPath, dest api.FSPathSpec) error {

	// Open the upload metadata and try to find the actual files in
	// the container.
	directory_listing, err := accessor.ReadDirWithOSPath(src)
	if err != nil {
		return err
	}

	for _, item := range directory_listing {
		if item.IsDir() {
			self.CopyDirectory(ctx, config_obj, scope, accessor,
				src.Append(item.Name()), dest.AddChild(item.Name()))
		} else {
			err = self.copyFile(ctx, config_obj, scope, accessor,
				src.Append(item.Name()),
				dest.AddChild(item.Name()).SetType(api.PATH_TYPE_DATASTORE_UNKNOWN))
			if err != nil {
				scope.Log("import_flow: %v", err)
				continue
			}
		}
	}

	return nil
}

func (self ImportCollectionFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "import_collection",
		Doc:      "Imports an offline collection zip file (experimental).",
		ArgType:  type_map.AddType(scope, &ImportCollectionFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.COLLECT_SERVER, acls.FILESYSTEM_READ).Build(),
	}
}

func (self ImportCollectionFunction) checkHuntInfo(
	root *accessors.OSPath, accessor accessors.FileSystemAccessor) (*api_proto.Hunt, error) {
	hunt_info := &api_proto.Hunt{}

	_, err := accessor.LstatWithOSPath(root.Append("hunt_info.json"))
	if err != nil {
		return nil, err
	}

	fd, err := accessor.OpenWithOSPath(root.Append("hunt_info.json"))
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	limitedReader := &io.LimitedReader{R: fd, N: BUFF_SIZE}
	data, err := ioutil.ReadAll(limitedReader)
	if err != nil && err != io.EOF {
		return nil, err
	}

	err = json.Unmarshal(data, hunt_info)

	return hunt_info, err
}

func init() {
	vql_subsystem.RegisterFunction(&ImportCollectionFunction{})
}
