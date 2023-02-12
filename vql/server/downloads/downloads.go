// VQL to produce exports of flows or hunts

package downloads

import (
	"context"
	"io"
	"io/ioutil"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/go-errors/errors"
	"github.com/sirupsen/logrus"
	"www.velocidex.com/golang/velociraptor/acls"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

var (
	Clock utils.Clock = utils.RealClock{}
)

type CreateFlowDownloadArgs struct {
	ClientId     string `vfilter:"required,field=client_id,doc=Client ID to export."`
	FlowId       string `vfilter:"required,field=flow_id,doc=The flow id to export."`
	Wait         bool   `vfilter:"optional,field=wait,doc=If set we wait for the download to complete before returning."`
	Type         string `vfilter:"optional,field=type,doc=Type of download to create (deperated Ignored)."`
	Template     string `vfilter:"optional,field=template,doc=Report template to use (deperated Ignored)."`
	Password     string `vfilter:"optional,field=password,doc=An optional password to encrypt the collection zip."`
	Format       string `vfilter:"optional,field=format,doc=Format to export (csv,json,csv_only) defaults to both."`
	ExpandSparse bool   `vfilter:"optional,field=expand_sparse,doc=If set we expand sparse files in the archive."`
	Name         string `vfilter:"optional,field=name,doc=If specified we call the file this name otherwise we generate name based on flow id."`
}

type CreateFlowDownload struct{}

func (self *CreateFlowDownload) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &CreateFlowDownloadArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("create_flow_download: %s", err.Error())
		return vfilter.Null{}
	}

	err = vql_subsystem.CheckAccess(scope, acls.PREPARE_RESULTS)
	if err != nil {
		scope.Log("create_flow_download: %s", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	format, err := reporting.GetContainerFormat(arg.Format)
	if err != nil {
		scope.Log("create_flow_download: %v", err)
		return vfilter.Null{}
	}

	principal := vql_subsystem.GetPrincipal(scope)
	logging.LogAudit(config_obj, principal, "create_flow_download",
		logrus.Fields{
			"format":    format,
			"client_id": arg.ClientId,
			"flow_id":   arg.FlowId,
		})

	result, err := createDownloadFile(
		ctx, scope, config_obj, format,
		arg.FlowId, arg.ClientId, arg.Password,
		arg.ExpandSparse, arg.Name, arg.Wait)
	if err != nil {
		scope.Log("create_flow_download: %s", err)
		return vfilter.Null{}
	}

	return result
}

func (self CreateFlowDownload) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "create_flow_download",
		Doc:     "Creates a download pack for the flow.",
		ArgType: type_map.AddType(scope, &CreateFlowDownloadArgs{}),
	}
}

type CreateHuntDownloadArgs struct {
	HuntId       string `vfilter:"required,field=hunt_id,doc=Hunt ID to export."`
	OnlyCombined bool   `vfilter:"optional,field=only_combined,doc=If set we only export combined results."`
	Wait         bool   `vfilter:"optional,field=wait,doc=If set we wait for the download to complete before returning."`
	Format       string `vfilter:"optional,field=format,doc=Format to export (csv,json) defaults to both."`
	Filename     string `vfilter:"optional,field=base,doc=Base filename to write to."`
	Password     string `vfilter:"optional,field=password,doc=An optional password to encrypt the collection zip."`
	ExpandSparse bool   `vfilter:"optional,field=expand_sparse,doc=If set we expand sparse files in the archive."`
}

type CreateHuntDownload struct{}

func (self *CreateHuntDownload) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &CreateHuntDownloadArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("create_hunt_download: %s", err.Error())
		return vfilter.Null{}
	}

	err = vql_subsystem.CheckAccess(scope, acls.PREPARE_RESULTS)
	if err != nil {
		scope.Log("create_hunt_download: %s", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	format, err := reporting.GetContainerFormat(arg.Format)
	if err != nil {
		scope.Log("create_hunt_download: %v", err)
		return vfilter.Null{}
	}

	result, err := createHuntDownloadFile(
		ctx, config_obj, scope, arg.HuntId,
		format, arg.ExpandSparse,
		arg.Wait, arg.OnlyCombined, arg.Filename, arg.Password)
	if err != nil {
		scope.Log("create_hunt_download: %s", err)
		return vfilter.Null{}
	}

	return result
}

func (self CreateHuntDownload) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "create_hunt_download",
		Doc:     "Creates a download pack for a hunt.",
		ArgType: type_map.AddType(scope, &CreateHuntDownloadArgs{}),
	}
}

func createDownloadFile(
	ctx context.Context,
	scope vfilter.Scope,
	config_obj *config_proto.Config,
	format reporting.ContainerFormat,
	flow_id, client_id, password string,
	expand_sparse bool,
	name string, wait bool) (api.FSPathSpec, error) {
	if client_id == "" || flow_id == "" {
		return nil, errors.New("Client Id and Flow Id should be specified.")
	}

	hostname := services.GetHostname(
		ctx, config_obj, client_id) + "-" + client_id
	flow_path_manager := paths.NewFlowPathManager(client_id, flow_id)
	download_file := flow_path_manager.GetDownloadsFile(hostname, password != "")
	if name != "" {
		download_file = flow_path_manager.GetDownloadsFileRawName(name)
	}

	logger := logging.GetLogger(config_obj, &logging.GUIComponent)
	logger.WithFields(logrus.Fields{
		"flow_id":       flow_id,
		"client_id":     client_id,
		"download_file": download_file,
	}).Info("CreateDownload")

	file_store_factory := file_store.GetFileStore(config_obj)
	fd, err := file_store_factory.WriteFile(download_file)
	if err != nil {
		return nil, err
	}

	err = fd.Truncate()
	if err != nil {
		return nil, err
	}

	// Create a new ZipContainer to write on. The container will close
	// the underlying writer.
	zip_writer, err := reporting.NewContainerFromWriter(
		config_obj, fd, password,
		reporting.DEFAULT_COMPRESSION, reporting.NO_METADATA)
	if err != nil {
		return nil, err
	}

	// zip_writer now owns fd and will close it when it closes below.

	// Report the progress as we write the container.
	progress_reporter := reporting.NewProgressReporter(config_obj,
		flow_path_manager.GetDownloadsStats(hostname, password != ""),
		download_file, zip_writer)

	wg := sync.WaitGroup{}
	wg.Add(1)

	// Write the bulk of the data asyncronously.
	go func() {
		defer wg.Done()
		defer progress_reporter.Close()

		// Will also close the underlying fd.
		defer zip_writer.Close()

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*600)
		defer cancel()

		err := downloadFlowToZip(ctx, scope, config_obj, format,
			client_id, path_specs.NewUnsafeFilestorePath(),
			flow_id, expand_sparse, zip_writer)
		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.GUIComponent)
			logger.Error("downloadFlowToZip: %v", err)
		}
	}()

	if wait {
		wg.Wait()
	}

	return download_file, nil
}

// Copies the collection into the zip file.
func downloadFlowToZip(
	ctx context.Context,
	scope vfilter.Scope,
	config_obj *config_proto.Config,
	format reporting.ContainerFormat,
	client_id string,
	prefix api.FSPathSpec,
	flow_id string,
	expand_sparse bool,
	zip_writer *reporting.Container) error {

	// Write the client info so it can be imported again
	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return err
	}

	client_info, err := client_info_manager.Get(ctx, client_id)
	if err == nil {
		err = zip_writer.WriteJSON(
			paths.ZipPathFromFSPathSpec(prefix.AddChild("client_info")),
			client_info)
		if err != nil {
			return err
		}
	}

	// Write the flow details.
	launcher, err := services.GetLauncher(config_obj)
	if err != nil {
		return err
	}

	flow_details, err := launcher.GetFlowDetails(config_obj, client_id, flow_id)
	if err == nil {
		err = zip_writer.WriteJSON(
			paths.ZipPathFromFSPathSpec(prefix.AddChild("collection_context")),
			flow_details)
		if err != nil {
			return err
		}
	}

	flow_requests, err := launcher.GetFlowRequests(config_obj,
		client_id, flow_id, 0, 100)
	if err == nil {
		err = zip_writer.WriteJSON(
			paths.ZipPathFromFSPathSpec(prefix.AddChild("requests")),
			flow_requests)
		if err != nil {
			return err
		}
	}

	// Copy the collection logs
	flow_path_manager := paths.NewFlowPathManager(client_id, flow_id)
	err = copyResultSetIntoContainer(ctx, config_obj, zip_writer, format,
		flow_path_manager.Log(), prefix.AddChild("logs"))
	if err != nil {
		return err
	}

	// Copy artifact results
	for _, name := range flow_details.Context.ArtifactsWithResults {
		artifact_path_manager, err := artifacts.NewArtifactPathManager(ctx,
			config_obj, client_id, flow_id, name)
		if err != nil {
			continue
		}

		err = copyResultSetIntoContainer(ctx, config_obj, zip_writer, format,
			artifact_path_manager.Path(), prefix.AddChild("results", name))
		if err != nil {
			return err
		}
	}

	// Copy uploads
	err = copyUploadFiles(ctx, scope, config_obj, zip_writer,
		prefix, format, flow_path_manager, expand_sparse)
	if err != nil {
		return err
	}
	return nil
}

func copyUploadFiles(
	ctx context.Context,
	scope vfilter.Scope,
	config_obj *config_proto.Config,
	container *reporting.Container,
	prefix api.FSPathSpec,
	format reporting.ContainerFormat,
	flow_path_manager *paths.FlowPathManager,
	expand_sparse bool) error {

	// Read all the upload metadata and copy the files to the container.
	file_store_factory := file_store.GetFileStore(config_obj)
	reader, err := result_sets.NewResultSetReader(file_store_factory,
		flow_path_manager.UploadMetadata())
	if err != nil {
		return err
	}

	output_chan := make(chan vfilter.Row)
	go func() {
		defer close(output_chan)
		defer reader.Close()

		for row := range reader.Rows(ctx) {
			components, pres := row.GetStrings("_Components")
			if !pres || len(components) < 1 {
				continue
			}

			dest := prefix.SetType(api.PATH_TYPE_FILESTORE_ANY)

			// Ensure we store index files into the correct place.
			file_type, _ := row.GetString("Type")
			if file_type == "idx" {
				// If we expand the files we dont need any indexes
				if expand_sparse {
					continue
				}
				dest = dest.SetType(api.PATH_TYPE_FILESTORE_SPARSE_IDX)
			}

			var src api.FSPathSpec

			if len(components) > 6 && components[0] == "clients" {
				//Remove the prefix in the file store where the files
				//are stored. The uploads file in the file store
				//refers to the location in the filestore where the
				//file is actually stored, while the uploads.json in
				//the container refers to the location in the
				//container where the file is actually
				//stored. Therefore we need to convert from one to the
				//other.

				// For example, in the file store a file may be
				// stored with these path components (root is the filestore):

				//	components = [
				//		"clients",
				//		"C.1bfa6928675831f5-O123",
				//		"collections",
				//		"F.CE2PSBS6BQCSO",
				//		"uploads",
				//		"auto",
				//		"C:",
				//		"Windows",
				//		"System32",
				//		"winevt",
				//		"Logs",
				//		"System.evtx"
				//	]

				//	In the container, we store a shorter path (root at the zip root)
				//	components = [
				//		"uploads",
				//		"auto",   <- accessor name
				//		"C:",
				//		"Windows",
				//		"System32",
				//		"winevt",
				//		"Logs",
				//		"System.evtx"
				//	]

				// Therefore we need to update the _Components field
				// to refer to the components in the zip file.

				row.Update("_Components", components[4:])
				src = path_specs.NewUnsafeFilestorePath(components...).
					SetType(api.PATH_TYPE_FILESTORE_ANY)
				dest = dest.AddChild(components[4:]...)

			} else {
				src = flow_path_manager.UploadContainer().AddChild(components[1:]...).
					SetType(api.PATH_TYPE_FILESTORE_ANY)
				dest = dest.AddChild(components...)
			}

			// Copy from the file store at these locations.
			err = copyFile(ctx, scope, config_obj, container,
				src, dest, expand_sparse)
			if err != nil {
				row.Set("Error", err.Error())
			}

			// Write the modified row into the uploads.json file.
			output_chan <- row
		}
	}()

	// Copy the modified rows into the uploads file.
	_, err = container.WriteResultSet(ctx, config_obj, scope, format,
		paths.ZipPathFromFSPathSpec(prefix.AddChild("uploads")), output_chan)

	return err
}

func copyFile(
	ctx context.Context,
	scope vfilter.Scope,
	config_obj *config_proto.Config,
	container *reporting.Container,
	src api.FSPathSpec,
	dest api.FSPathSpec,
	expand_sparse bool) (err error) {

	scope.Log("DEBUG: downloadFlowToZip: Copy file from %v to %v\n",
		src.AsClientPath(), dest.AsClientPath())

	file_store_factory := file_store.GetFileStore(config_obj)
	fd, err := file_store_factory.ReadFile(src)
	if err != nil {
		return err
	}
	defer fd.Close()

	out_fd, err := container.Create(
		paths.ZipPathFromFSPathSpec(dest), Clock.Now())
	if err != nil {
		return err
	}
	defer out_fd.Close()

	reader := fd.(io.ReadSeeker)

	if expand_sparse {
		reader = maybeExpandSparseFile(ctx, scope, config_obj, src, fd)
	}

	_, err = utils.Copy(ctx, out_fd, reader)
	return err
}

// Check for an index file in the filestore and expand the file if we
// find it. This can be very large!
func maybeExpandSparseFile(
	ctx context.Context,
	scope vfilter.Scope,
	config_obj *config_proto.Config,
	src api.FSPathSpec,
	reader io.ReadSeeker) io.ReadSeeker {

	file_store_factory := file_store.GetFileStore(config_obj)

	// Try to read the index ranges
	idx_fd, err := file_store_factory.ReadFile(src.
		SetType(api.PATH_TYPE_FILESTORE_SPARSE_IDX))
	if err != nil {
		return reader
	}
	defer idx_fd.Close()

	serialized, err := ioutil.ReadAll(idx_fd)
	if err != nil {
		return reader
	}

	index := &actions_proto.Index{}
	err = json.Unmarshal(serialized, &index)
	if err != nil {
		return reader
	}

	logger := logging.GetLogger(config_obj, &logging.GUIComponent)

	// If the file is too sparse forget about it.
	if !uploads.ShouldPadFile(config_obj, index) {
		logger.Debug("File %v is too sparse - unable to expand it.", src)
		scope.Log("File %v is too sparse - unable to expand it.", src)
		return reader
	}

	scope.Log("File %v is sparse - expanding.", src)
	logger.Debug("File %v is sparse - expanding.", src)
	return utils.NewReadSeekReaderAdapter(&utils.RangedReader{
		ReaderAt: utils.MakeReaderAtter(reader),
		Index:    index,
	})
}

func copyResultSetIntoContainer(
	ctx context.Context,
	config_obj *config_proto.Config,
	container *reporting.Container,
	format reporting.ContainerFormat,
	src api.FSPathSpec,
	dest api.FSPathSpec) (err error) {

	file_store_factory := file_store.GetFileStore(config_obj)
	reader, err := result_sets.NewResultSetReader(file_store_factory, src)
	if err != nil {
		return err
	}
	defer reader.Close()

	json_writer, csv_writer := getWriters(dest, format, container)
	defer maybeClose(json_writer)
	defer maybeClose(csv_writer)

	buf_chan, err := reader.JSON(ctx)
	if err != nil {
		reader.Close()
		return
	}

	json.ConvertJSONL(buf_chan, json_writer, csv_writer, nil)

	return nil
}

func getWriters(
	path api.FSPathSpec,
	format reporting.ContainerFormat,
	zip_writer *reporting.Container) (json_writer, csv_writer io.WriteCloser) {

	if format&reporting.ContainerFormatJson > 0 {
		json_writer, _ = zip_writer.Create(
			paths.ZipPathFromFSPathSpec(
				path.SetType(api.PATH_TYPE_FILESTORE_JSON)),
			time.Time{})
	}

	if format&reporting.ContainerFormatCSV > 0 {
		csv_writer, _ = zip_writer.Create(
			paths.ZipPathFromFSPathSpec(
				path.SetType(api.PATH_TYPE_FILESTORE_CSV)),
			time.Time{})
	}

	return json_writer, csv_writer
}

func maybeClose(fd io.WriteCloser) {
	if fd != nil {
		fd.Close()
	}
}

func createHuntDownloadFile(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	hunt_id string,
	format reporting.ContainerFormat,
	expand_sparse bool,
	wait, only_combined bool,
	base_filename, password string) (api.FSPathSpec, error) {
	if hunt_id == "" {
		return nil, errors.New("Hunt Id should be specified.")
	}

	logger := logging.GetLogger(config_obj, &logging.GUIComponent)
	hunt_path_manager := paths.NewHuntPathManager(hunt_id)
	download_file := hunt_path_manager.GetHuntDownloadsFile(
		only_combined, base_filename, password != "")

	logger.WithFields(logrus.Fields{
		"hunt_id":       hunt_id,
		"download_file": download_file,
	}).Info("CreateHuntDownload")

	// Write the download file
	file_store_factory := file_store.GetFileStore(config_obj)
	fd, err := file_store_factory.WriteFile(download_file)
	if err != nil {
		return nil, err
	}
	// fd is closed by the zip container below.

	err = fd.Truncate()
	if err != nil {
		fd.Close()
		return nil, err
	}

	hunt_dispatcher, err := services.GetHuntDispatcher(config_obj)
	if err != nil {
		fd.Close()
		return nil, err
	}

	hunt_details, pres := hunt_dispatcher.GetHunt(hunt_id)
	if !pres {
		fd.Close()
		return nil, errors.New("Hunt not found")
	}

	// Do these first to ensure errors are returned if the zip file
	// is not writable.
	zip_writer, err := reporting.NewContainerFromWriter(
		config_obj, fd, password, 5, nil /* metadata */)
	if err != nil {
		fd.Close()
		return nil, err
	}

	// zip_writer now owns fd and will close it when it closes below.

	// Report the progress as we write the container.
	progress_reporter := reporting.NewProgressReporter(config_obj,
		hunt_path_manager.GetHuntDownloadsStats(only_combined,
			base_filename, password != ""),
		download_file, zip_writer)

	wg := sync.WaitGroup{}
	wg.Add(1)

	// Write the bulk of the data asyncronously.
	go func() {
		defer wg.Done()
		defer progress_reporter.Close()

		// Will also close the underlying fd.
		defer zip_writer.Close()

		// Allow one hour to write the zip
		sub_ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
		defer cancel()

		err = generateCombinedResults(
			sub_ctx, config_obj, scope,
			hunt_details, format, zip_writer)
		if err != nil {
			logger.Error("createHuntDownloadFile: %v", err)
			return
		}

		// If the user only asked for combined results do not
		// export specific flow.
		if only_combined {
			return
		}

		for flow_details := range hunt_dispatcher.GetFlows(sub_ctx,
			config_obj, scope, hunt_id, 0) {

			if flow_details == nil || flow_details.Context == nil {
				continue
			}

			flow_id := flow_details.Context.SessionId
			client_id := flow_details.Context.ClientId

			if flow_id == "" || client_id == "" {
				continue
			}

			hostname := services.GetHostname(
				sub_ctx, config_obj, client_id) + "-" + client_id
			err := downloadFlowToZip(
				sub_ctx, scope, config_obj, format, client_id,
				path_specs.NewUnsafeFilestorePath(hostname),
				flow_id, expand_sparse, zip_writer)
			if err != nil {
				logging.GetLogger(config_obj, &logging.FrontendComponent).
					WithFields(logrus.Fields{
						"hunt_id": hunt_id,
						"error":   err.Error(),
						"bt":      logging.GetStackTrace(err),
					}).Info("DownloadHuntResults")
				continue
			}
		}
	}()

	if wait {
		wg.Wait()
	}

	return download_file, nil
}

func generateCombinedResults(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	hunt_details *api_proto.Hunt,
	format reporting.ContainerFormat,
	zip_writer *reporting.Container) error {

	file_store_factory := file_store.GetFileStore(config_obj)
	hunt_dispatcher, err := services.GetHuntDispatcher(config_obj)
	if err != nil {
		return err
	}

	indexer, err := services.GetIndexer(config_obj)
	if err != nil {
		return err
	}

	// Export aggregate CSV and JSON files for all clients.
	for _, artifact_source := range hunt_details.ArtifactSources {
		// Figure out where to write it
		path_manager := path_specs.NewUnsafeFilestorePath(
			"results", "All "+artifact_source)

		json_writer, csv_writer := getWriters(path_manager, format, zip_writer)
		defer maybeClose(json_writer)
		defer maybeClose(csv_writer)

		for flow_details := range hunt_dispatcher.GetFlows(ctx,
			config_obj, scope, hunt_details.HuntId, 0) {

			if flow_details == nil || flow_details.Context == nil {
				continue
			}

			flow_id := flow_details.Context.SessionId
			client_id := flow_details.Context.ClientId

			path_manager := artifacts.NewArtifactPathManagerWithMode(
				config_obj, client_id, flow_id, artifact_source,
				paths.MODE_CLIENT)

			reader, err := result_sets.NewResultSetReader(
				file_store_factory, path_manager.Path())
			if err != nil {
				continue
			}

			buf_chan, err := reader.JSON(ctx)
			if err != nil {
				reader.Close()
				continue
			}

			fqdn := ""
			api_client, err := indexer.FastGetApiClient(
				ctx, config_obj, client_id)
			if err == nil && api_client != nil && api_client.OsInfo != nil {
				fqdn = api_client.OsInfo.Fqdn
			}

			json.ConvertJSONL(buf_chan, json_writer, csv_writer,
				ordereddict.NewDict().
					Set("FlowId", flow_id).
					Set("ClientId", client_id).
					Set("Fqdn", fqdn))

			reader.Close()
		}
	}

	return nil
}

func init() {
	vql_subsystem.RegisterFunction(&CreateHuntDownload{})
	vql_subsystem.RegisterFunction(&CreateFlowDownload{})
}
