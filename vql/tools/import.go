package tools

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/api"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/result_sets"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/paths"
	artifact_paths "www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type ImportCollectionFunctionArgs struct {
	ClientId string `vfilter:"required,field=client_id,doc=The client id to import to. Use 'auto' to generate a new client id."`
	Hostname string `vfilter:"optional,field=hostname,doc=When creating a new client, set this as the hostname."`
	Filename string `vfilter:"required,field=filename,doc=Path on server to the collector zip."`
	Accessor string `vfilter:"optional,field=accessor,doc=The accessor to use"`
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
	err = vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("import_collection: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		scope.Log("import_collection: %v", err)
		return vfilter.Null{}
	}

	if arg.ClientId == "auto" {
		arg.ClientId, err = makeNewClient(config_obj, arg.Hostname)
		if err != nil {
			scope.Log("import_collection: %v", err)
			return vfilter.Null{}
		}
	}

	api_client, err := api.GetApiClient(ctx,
		config_obj, nil, arg.ClientId,
		false /* detailed */)
	if err != nil || api_client.AgentInformation == nil ||
		api_client.AgentInformation.Name == "" {
		scope.Log("import_collection: client_id not known")
		return vfilter.Null{}
	}

	file_store_factory := file_store.GetFileStore(config_obj)
	manager, err := services.GetRepositoryManager()
	if err != nil {
		scope.Log("import_collection: %v", err)
		return vfilter.Null{}
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		scope.Log("import_collection: %v", err)
		return vfilter.Null{}
	}

	// Open the zip file we are importing.
	accessor, err := glob.GetAccessor(arg.Accessor, scope)
	if err != nil {
		scope.Log("import_collection: %v", err)
		return vfilter.Null{}
	}

	fd, err := accessor.Open(arg.Filename)
	if err != nil {
		scope.Log("import_collection: %v", err)
		return vfilter.Null{}
	}
	defer fd.Close()

	st, err := accessor.Lstat(arg.Filename)
	if err != nil {
		scope.Log("import_collection: %v", err)
		return vfilter.Null{}
	}

	zipfile, err := zip.NewReader(utils.ReaderAtter{fd}, st.Size())
	if err != nil {
		scope.Log("import_collection: %v", err)
		return vfilter.Null{}
	}

	// Keep track of all the artifacts in the zip file.
	artifacts := make(map[string]bool)

	// Create a new flow and path manager for it.
	flow_id := launcher.NewFlowId(arg.ClientId)
	path_manager := paths.NewFlowPathManager(arg.ClientId, flow_id)
	new_flow := &flows_proto.ArtifactCollectorContext{
		SessionId: flow_id,
		ClientId:  arg.ClientId,
		Request: &flows_proto.ArtifactCollectorArgs{
			Creator:  vql_subsystem.GetPrincipal(scope),
			ClientId: arg.ClientId,
		},
		CreateTime: uint64(time.Now().UnixNano() / 1000),
		State:      flows_proto.ArtifactCollectorContext_FINISHED,
	}

	uploaded_files_result_set, err := result_sets.NewResultSetWriter(
		file_store_factory, path_manager.UploadMetadata(),
		nil, true /* truncate */)
	if err != nil {
		scope.Log("import_collection: %v", err)
		return vfilter.Null{}
	}
	defer uploaded_files_result_set.Close()

	log_result_set, err := result_sets.NewResultSetWriter(
		file_store_factory, path_manager.Log(),
		nil, true /* truncate */)
	if err != nil {
		scope.Log("import_collection: %v", err)
		return vfilter.Null{}
	}
	defer log_result_set.Close()

	// A log function that stores messages in the flow log as well
	// as print them to the screen.
	log := func(format string, args ...interface{}) {
		now := time.Now().UTC()
		log_result_set.Write(ordereddict.NewDict().
			Set("Timestamp", fmt.Sprintf("%v", now)).
			Set("time", time.Unix(int64(now.UnixNano())/1000000, 0).String()).
			Set("message", fmt.Sprintf(format, args...)))

		scope.Log(format, args...)
	}

	log("Importing zip file %v into client id %v", arg.Filename, arg.ClientId)

	for _, file := range zipfile.File {
		log("Filename %v", file.Name)

		// Files can be either an artifact or an upload
		artifact_name := strings.TrimSuffix(file.Name, ".json")
		artifact, pres := repository.Get(config_obj, artifact_name)
		if pres {
			// File is an artifact result set - import it
			// into the filestore. artifact_name is the
			// full name include source of the artifact so
			// we need to dedup here.
			artifacts[artifact.Name] = true

			new_flow.ArtifactsWithResults = append(new_flow.ArtifactsWithResults,
				artifact_name)

			func() {
				// Now copy the artifact results over.
				fd, err := file.Open()
				if err != nil {
					log("Error copying %v", err)
					return
				}
				defer fd.Close()

				artifact_path_manager := artifact_paths.NewArtifactPathManager(
					config_obj, arg.ClientId, flow_id, artifact_name)

				rs_writer, err := result_sets.NewResultSetWriter(
					file_store_factory, artifact_path_manager,
					nil, true /* truncate */)
				if err != nil {
					log("Error copying %v", err)
					return
				}
				defer rs_writer.Close()

				// Now copy the rows from the zip to the filestore.
				count := 0
				for row := range utils.ReadJsonFromFile(ctx, fd) {
					new_flow.TotalCollectedRows++
					rs_writer.Write(row)
					count++
				}

				log("Imported %v rows", count)
			}()
		} else {
			new_flow.TotalUploadedFiles++
			new_flow.TotalUploadedBytes += file.UncompressedSize64

			func() {
				now := time.Now()
				fd, err := file.Open()
				if err != nil {
					log("Error copying %v", err)
					return
				}
				defer fd.Close()

				out_path := path_manager.GetUploadsFile("file", file.Name).Path()
				out_fd, err := file_store_factory.WriteFile(out_path)
				defer out_fd.Close()

				log("Copying file %v -> %v", file.Name, out_path)

				_, err = utils.Copy(ctx, out_fd, fd)
				if err != nil {
					log("Error copying %v", err)
				}

				uploaded_files_result_set.Write(ordereddict.NewDict().
					Set("Timestamp", now.UTC().Unix()).
					Set("started", now.UTC().String()).
					Set("vfs_path", out_path).
					Set("file_size", file.UncompressedSize64).
					Set("uploaded_size", file.UncompressedSize64))
			}()
		}
	}

	// Copy all unique artifacts to the request struct - this will
	// go into the flow context.
	for k := range artifacts {
		new_flow.Request.Artifacts = append(new_flow.Request.Artifacts, k)
	}

	err = db.SetSubject(config_obj, path_manager.Path(), new_flow)
	if err != nil {
		scope.Log("import_collection: %v", err)
		return vfilter.Null{}
	}

	// Generate a fake System.Flow.Completion event for the
	// uploaded flow in case there are any listeners who are
	// interested.
	journal, err := services.GetJournal()
	if err != nil {
		scope.Log("import_collection: %v", err)
		return vfilter.Null{}
	}

	row := ordereddict.NewDict().
		Set("Timestamp", time.Now().UTC().Unix()).
		Set("Flow", new_flow).
		Set("FlowId", new_flow.SessionId).
		Set("ClientId", new_flow.ClientId)

	err = journal.PushRowsToArtifact(config_obj,
		[]*ordereddict.Dict{row},
		"System.Flow.Completion", new_flow.ClientId,
		new_flow.SessionId,
	)

	return new_flow
}

func (self ImportCollectionFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "import_collection",
		Doc:     "Imports an offline collection zip file (experimental).",
		ArgType: type_map.AddType(scope, &ImportCollectionFunctionArgs{}),
	}
}

// Generate a new client id
func NewClientId() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	dst := make([]byte, hex.EncodedLen(8))
	hex.Encode(dst, buf)
	return "C." + string(dst)
}

// Create a new client record
func makeNewClient(
	config_obj *config_proto.Config,
	hostname string) (string, error) {

	if hostname == "" {
		return "", errors.New("New clients must have a hostname")
	}

	client_id := NewClientId()
	client_info := &actions_proto.ClientInfo{
		ClientId:     client_id,
		Hostname:     hostname,
		Fqdn:         hostname,
		Architecture: "Offline",
		ClientName:   "OfflineVelociraptor",
	}

	db, err := datastore.GetDB(config_obj)
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
	keywords := []string{
		"all", // This is used for "." search
		client_id,
		client_info.Hostname,
		client_info.Fqdn,
		"host:" + client_info.Hostname,
	}

	return client_id, db.SetIndex(config_obj,
		constants.CLIENT_INDEX_URN,
		client_id, keywords,
	)
}

func init() {
	vql_subsystem.RegisterFunction(&ImportCollectionFunction{})
}
