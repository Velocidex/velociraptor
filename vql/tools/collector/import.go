package collector

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
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
	BUFF_SIZE     = 1000000
	GuessClientId = ""
	GuessHostname = ""
	ImportAHunt   = true
)

type ImportCollectionFunctionArgs struct {
	ClientId   string `vfilter:"optional,field=client_id,doc=The client id to import to. Use 'auto' to generate a new client id or use the host info from the collection."`
	Hostname   string `vfilter:"optional,field=hostname,doc=When creating a new client, set this as the hostname."`
	Filename   string `vfilter:"required,field=filename,doc=Path on server to the collector zip."`
	Accessor   string `vfilter:"optional,field=accessor,doc=The accessor to use."`
	ImportType string `vfilter:"optional,field=import_type,doc=Whether the import is an offline_collector or hunt."`
}

type ImportCollectionFunction struct{}

func (self ImportCollectionFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "import_collection", args)()

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

	err = root.SetPathSpec(&accessors.PathSpec{
		DelegateAccessor: arg.Accessor,
		DelegatePath:     arg.Filename,
	})
	if err != nil {
		scope.Log("import_collection: %v", err)
		return vfilter.Null{}
	}

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
		flow, err := self.importFlow(
			ctx, scope, config_obj,
			accessor, root, arg.ClientId,
			arg.Hostname, !ImportAHunt)
		if err != nil {
			scope.Log("import_collection: %v", err)
			return vfilter.Null{}
		}
		return flow
	}

	if arg.ImportType == "hunt" {
		hunt_obj, err := self.importHunt(ctx, scope, config_obj, root, accessor)
		if err != nil {
			scope.Log("import_collection: importHunt: %v", err)
			return vfilter.Null{}
		}
		return hunt_obj
	}

	return vfilter.Null{}
}

func (self ImportCollectionFunction) importHunt(
	ctx context.Context,
	scope vfilter.Scope,
	config_obj *config_proto.Config,
	root *accessors.OSPath,
	accessor accessors.FileSystemAccessor,
) (*api_proto.Hunt, error) {
	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return nil, err
	}
	// Check if there is a hunt_info.json. This won't work with
	// older exports (<0.7.1) because we previously didn't export
	// all the hunt information.
	hunt_info, err := self.checkHuntInfo(root, accessor)
	if err != nil {
		return nil, err
	}

	// Update the huntId in case it was already taken.
	hunt_info.HuntId, err = self.importHuntObject(ctx, scope, config_obj, hunt_info)
	if err != nil {
		return nil, err
	}

	directory_listing, err := accessor.ReadDirWithOSPath(root)
	if err != nil {
		return nil, err
	}

	for _, item := range directory_listing {
		if !item.IsDir() || item.Name() == "results" || item.Name() == "uploads" {
			continue
		}

		path := root.Append(item.Name())

		// Import the flow into the system
		flow, err := self.importFlow(
			ctx, scope, config_obj,
			accessor, path,
			GuessClientId, GuessHostname,
			ImportAHunt)

		// And now add it to the hunt.
		if err != nil {
			scope.Log("import_collection: importFlow: %v", err)
			continue
		}

		_ = journal.PushRowsToArtifact(ctx, config_obj,
			[]*ordereddict.Dict{ordereddict.NewDict().
				Set("HuntId", hunt_info.HuntId).
				Set("mutation", &api_proto.HuntMutation{
					HuntId: hunt_info.HuntId,
					Assignment: &api_proto.FlowAssignment{
						ClientId: flow.ClientId,
						FlowId:   flow.SessionId,
					},
				})},
			"Server.Internal.HuntModification", flow.ClientId, "")
	}

	return hunt_info, nil
}

func (self ImportCollectionFunction) importFlow(
	ctx context.Context,
	scope vfilter.Scope,
	config_obj *config_proto.Config,
	accessor accessors.FileSystemAccessor,
	root *accessors.OSPath,
	client_id string,
	hostname string,
	hunt bool) (*flows_proto.ArtifactCollectorContext, error) {

	collection_context := &flows_proto.ArtifactCollectorContext{}
	path := root.Append("collection_context.json")

	err := self.getFile(accessor, path, collection_context)
	if err != nil || collection_context.SessionId == "" {
		// Support older collections which were encoded a bit
		// differently.
		details := &api_proto.FlowDetails{}
		err := self.getFile(accessor, path, details)
		if err != nil {
			return nil, fmt.Errorf("unable to load collection_context: %v", err)
		}
		collection_context = details.Context
	}

	if client_id == "auto" {
		client_id = ""
	}

	client_id, err = self.getClientIdFromHostnameOrCollection(
		ctx, scope, config_obj, client_id, hostname, root, accessor)
	if err != nil {
		return nil, err
	}

	// Update the collection_context to refer to the new client id
	collection_context.ClientId = client_id

	launcher, err := services.GetLauncher(config_obj)
	if err != nil {
		return nil, err
	}

	// Check if this flow is already in this client. If it is then we
	// make a new flow id so the new import is kept separated.
	_, err = launcher.GetFlowDetails(
		ctx, config_obj, services.GetFlowOptions{},
		client_id, collection_context.SessionId)
	if err == nil {
		collection_context.SessionId = utils.NewFlowId(client_id)
	}

	// Write the flow and update indexes
	err = launcher.Storage().WriteFlow(ctx, config_obj,
		collection_context, utils.BackgroundWriter)
	if err != nil {
		return nil, err
	}

	err = launcher.Storage().WriteFlowIndex(ctx, config_obj,
		collection_context)
	if err != nil {
		return nil, err
	}

	// Copy the requests for provenance
	tasks := &api_proto.ApiFlowRequestDetails{}

	// If there is no requests.json, just write an empty one
	err = self.getFile(accessor, root.Append("requests.json"), tasks)
	if err != nil || len(tasks.Items) == 0 {
		tasks.Items = append(tasks.Items, &crypto_proto.VeloMessage{})
	}

	err = launcher.Storage().WriteTask(ctx, config_obj, client_id,
		tasks.Items[0])
	if err != nil {
		return nil, err
	}

	// Copy the logs results set over.
	flow_path_manager := paths.NewFlowPathManager(client_id,
		collection_context.SessionId)
	err = self.copyResultSet(ctx, config_obj, scope,
		accessor, root.Append("log.json"), flow_path_manager.Log(),
		PassThroughTransform)
	if err != nil {
		// Support older containers who had this spelled different
		err = self.copyResultSet(ctx, config_obj, scope,
			accessor, root.Append("logs.json"), flow_path_manager.Log(),
			PassThroughTransform)
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
			artifact_path_manager.Path(),
			PassThroughTransform)
		if err != nil {
			scope.Log("import_flow: %v", err)
		}
	}

	// Now copy any uploads - first get the metadata.
	err = self.copyResultSet(ctx, config_obj, scope,
		accessor, root.Append("uploads.json"),
		flow_path_manager.UploadMetadata(),
		self.UploadMetadataTransform(ctx, config_obj, scope,
			accessor, root, flow_path_manager))

	// It is ok that there are no uploads - just ignore it.
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
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
		return nil, err
	}
	err = journal.PushRowsToArtifact(ctx, config_obj,
		[]*ordereddict.Dict{row},
		"System.Flow.Completion", collection_context.ClientId,
		collection_context.SessionId,
	)

	return collection_context, err
}

func (self ImportCollectionFunction) importHuntObject(
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
		_, pres := hunt_disp.GetHunt(ctx, hunt.HuntId)
		if pres {
			hunt.HuntId = hunt_dispatcher.GetNewHuntId()
		}
	}

	// Create the new hunt in the stopped state so it does not
	// dispatch new clients.
	hunt.State = api_proto.Hunt_STOPPED
	hunt.OrgIds = nil

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

// Ensure the client record exists, if not create it.
func (self ImportCollectionFunction) ensureClientId(
	ctx context.Context,
	scope vfilter.Scope,
	config_obj *config_proto.Config,
	client_id string,
	hostname string) error {

	// Check if the client is already known.
	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return err
	}

	// Check to see if we know about this client id. If we do
	// then just return the same client id as in the
	// container.
	_, err = client_info_manager.Get(ctx, client_id)
	if err == nil {
		return nil
	}

	// If we get here we dont know the client so we just create a
	// new one with this client id

	// Client is not known, create it.
	clients.NewClientFunction{}.Call(ctx, scope, ordereddict.NewDict().
		Set("client_id", client_id).
		Set("first_seen_at", time.Now()).
		Set("last_seen_at", time.Now()).
		Set("hostname", hostname))

	return nil
}

// Reads the client_info.json and attempts to find a client id that
// would work.
// This is based on the following logic:
//  1. If the user provides a client_id then we use that.
//  2. If the collection contains a client id which already exists on
//     this server we use that.
//  3. If there is no exact client id on this server but there is a
//     client with the same host id or hostname, we use that instead.
//  4. Finally we create a new client with a new client id to contain
//     the import.
func (self ImportCollectionFunction) getClientIdFromHostnameOrCollection(
	ctx context.Context,
	scope types.Scope,
	config_obj *config_proto.Config,
	client_id string,
	hostname string,
	root *accessors.OSPath,
	accessor accessors.FileSystemAccessor) (string, error) {

	// Try to get the host info from the collection.
	host_info := ordereddict.NewDict().SetCaseInsensitive()
	path := root.Append("client_info.json")
	err := self.getFile(accessor, path, host_info)
	if err == nil {
		// Override the hostname with the one in the collection.
		collection_hostname, pres := host_info.GetString("hostname")
		if pres {
			hostname = collection_hostname
		}

		// We dont know this client id - Search for a client id we do
		// know, that has the same hostname. This happens in importing
		// the offline collection which does not contain a client id.
		if client_id == "" && hostname != "" {
			indexer, err := services.GetIndexer(config_obj)
			if err != nil {
				return "", err
			}

			scope.Log("Searching for a client id with hostname '%v'", hostname)

			search_resp, err := indexer.SearchClients(ctx, config_obj,
				&api_proto.SearchClientsRequest{
					Query: "host:" + hostname,
				}, "")

			if err == nil {
				for _, resp := range search_resp.Items {
					if strings.EqualFold(resp.OsInfo.Hostname, hostname) {
						scope.Log("client id found '%v'", resp.ClientId)
						return resp.ClientId, nil
					}
				}
			}
		}

		// No client id - create one based on the host id
		if client_id == "" {
			host_id, pres := host_info.GetString("HostID")
			if pres && host_id != "" {
				// Make the client id based on the host id. This is used
				// to ensure that the client id is consistent each time
				// the offline collector is run on the same endpoint.
				client_id = "C." + strings.TrimPrefix(host_id, "C.")
			}
		}

		// Just use the client id stored in the collection.
		if client_id == "" {
			client_id, _ = host_info.GetString("client_id")
		}

		if client_id != "" {
			scope.Log(
				"Found client_info.json file in collection: "+
					"Using client id '%v' and hostname '%v'",
				client_id, hostname)
		}
	}

	// If we get here the collection does not have a client_info.json
	// file - this should not happen unless the collection is very
	// old!
	if client_id == "" {
		client_id = clients.NewClientId()
		scope.Log("Creating a new client id '%v'", client_id)
	}
	return client_id, self.ensureClientId(
		ctx, scope, config_obj, client_id, hostname)
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

	if utils.IsNil(res) {
		return errors.New("Failed to create new client.")
	}

	return nil
}

func (self ImportCollectionFunction) copyResultSet(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	accessor accessors.FileSystemAccessor,
	src *accessors.OSPath, dest api.FSPathSpec,
	transformer func(json []byte) []byte) error {

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

			buffer.Write(transformer(row_data))
			count++

			// Dump chunks into the result set - this is much faster
			// than parsing the encoding.
			if count > 100 {
				flush()
			}
		}
	}
}

func (self ImportCollectionFunction) getFile(
	accessor accessors.FileSystemAccessor,
	path *accessors.OSPath, target interface{}) error {

	fd, err := accessor.OpenWithOSPath(path)
	if err != nil {
		return err
	}
	defer fd.Close()

	data, err := utils.ReadAllWithLimit(fd, constants.MAX_MEMORY)
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

	err = out_fd.Truncate()
	if err != nil {
		return err
	}

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

func (self ImportCollectionFunction) checkHuntInfo(
	root *accessors.OSPath, accessor accessors.FileSystemAccessor) (*api_proto.Hunt, error) {
	hunt_info := &api_proto.Hunt{}

	fd, err := accessor.OpenWithOSPath(root.Append("hunt_info.json"))
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	data, err := utils.ReadAllWithLimit(fd, constants.MAX_MEMORY)
	if err != nil && err != io.EOF {
		return nil, err
	}

	err = json.Unmarshal(data, hunt_info)

	return hunt_info, err
}

func PassThroughTransform(x []byte) []byte {
	return x
}

// The collection zip file treats uploads relative to the zip file but
// when we store them into the filestore they are normally rooted at
// the flow_path_manager so we need to adjust the components.
func (self ImportCollectionFunction) UploadMetadataTransform(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	accessor accessors.FileSystemAccessor,
	root *accessors.OSPath,
	flow_path_manager *paths.FlowPathManager) func(in []byte) []byte {

	base := flow_path_manager.UploadContainer()

	// Callback is invoked for each upload in the zip file.
	return func(in []byte) []byte {
		row, err := utils.ParseJsonToObject(in)
		if err != nil {
			// Line is not valid, drop it.
			return nil
		}

		components, pres := row.GetStrings("_Components")
		if !pres || len(components) == 0 || components[0] != "uploads" {
			// Drop the line as the upload is not valid.
			return nil
		}

		// Now copy the file from the zip into the filestore.

		// First directory in zip file is "upload" we skip that
		// and append the other components to the filestore path.
		dest := base.AddChild(components[1:]...)

		// Copy from the archive to the file store at these locations.
		src := root.Append(components...)

		// Do not copy index files specifically - the index file
		// will be copied as part of the file it belongs to.
		row_type, _ := row.GetString("Type")
		if row_type != "idx" {
			err := self.copyFileWithIndex(ctx, config_obj, scope,
				accessor, src, dest)
			if err != nil {
				scope.Log("import_flow: %v", err)
			}
		}

		row.Update("_Components", dest.Components())
		serialized, err := row.MarshalJSON()
		if err == nil {
			serialized = append(serialized, '\n')
			return serialized
		}

		return in
	}
}

func init() {
	vql_subsystem.RegisterFunction(&ImportCollectionFunction{})
}
