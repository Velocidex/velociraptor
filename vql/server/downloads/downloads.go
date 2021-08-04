package downloads

import (
	"archive/zip"
	"context"
	"io"
	"io/ioutil"
	"os"
	"path"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/actions"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/flows"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	artifact_paths "www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type CreateFlowDownloadArgs struct {
	ClientId string `vfilter:"required,field=client_id,doc=Client ID to export."`
	FlowId   string `vfilter:"required,field=flow_id,doc=The flow id to export."`
	Wait     bool   `vfilter:"optional,field=wait,doc=If set we wait for the download to complete before returning."`
	Type     string `vfilter:"optional,field=type,doc=Type of download to create (e.g. 'report') default a full zip file."`
	Template string `vfilter:"optional,field=template,doc=Report template to use (defaults to Reporting.Default)."`
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

	if arg.Template == "" {
		arg.Template = "Reporting.Default"
	}

	switch arg.Type {
	case "report":
		result, err := CreateFlowReport(
			config_obj, scope, arg.FlowId, arg.ClientId,
			arg.Template, arg.Wait)
		if err != nil {
			scope.Log("create_flow_download: %s", err)
			return vfilter.Null{}
		}
		return result

	case "":
		result, err := createDownloadFile(config_obj, arg.FlowId, arg.ClientId, arg.Wait)
		if err != nil {
			scope.Log("create_flow_download: %s", err)
			return vfilter.Null{}
		}
		return result

	default:
		scope.Log("Unknown report type %v", arg.Type)
	}

	return vfilter.Null{}
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

	var write_csv, write_json bool

	switch arg.Format {
	case "json":
		write_json = true

	case "csv":
		write_csv = true

	case "":
		write_json = true
		write_csv = true

	default:
		scope.Log("Unknown format parameter either 'json', 'cvs' or empty for both.")
		return vfilter.Null{}
	}

	result, err := createHuntDownloadFile(
		ctx, config_obj, scope, arg.HuntId,
		write_json, write_csv,
		arg.Wait, arg.OnlyCombined, arg.Filename)
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
	config_obj *config_proto.Config,
	flow_id, client_id string,
	wait bool) (api.FSPathSpec, error) {
	if client_id == "" || flow_id == "" {
		return nil, errors.New("Client Id and Flow Id should be specified.")
	}

	hostname := services.GetHostname(client_id)
	flow_path_manager := paths.NewFlowPathManager(client_id, flow_id)
	download_file := flow_path_manager.GetDownloadsFile(hostname)

	logger := logging.GetLogger(config_obj, &logging.GUIComponent)
	logger.WithFields(logrus.Fields{
		"flow_id":       flow_id,
		"client_id":     client_id,
		"download_file": download_file,
	}).Error("CreateDownload")

	file_store_factory := file_store.GetFileStore(config_obj)
	fd, err := file_store_factory.WriteFile(download_file)
	if err != nil {
		return nil, err
	}

	err = fd.Truncate()
	if err != nil {
		return nil, err
	}

	lock_file_spec := download_file.SetType(api.PATH_TYPE_FILESTORE_LOCK)
	lock_file, err := file_store_factory.WriteFile(lock_file_spec)
	if err != nil {
		return nil, err
	}
	lock_file.Close()

	flow_details, err := flows.GetFlowDetails(config_obj, client_id, flow_id)
	if err != nil {
		return nil, err
	}

	// Do these first to ensure errors are returned if the zip file
	// is not writable.
	zip_writer := zip.NewWriter(fd)
	f, err := zip_writer.Create("FlowDetails")
	if err != nil {
		fd.Close()
		return nil, err
	}

	flow_details_json, _ := json.ConvertProtoToOrderedDict(flow_details).MarshalJSON()
	_, err = f.Write(flow_details_json)
	if err != nil {
		zip_writer.Close()
		fd.Close()
		return nil, err
	}

	wg := sync.WaitGroup{}
	wg.Add(1)

	// Write the bulk of the data asyncronously.
	go func() {
		defer wg.Done()
		defer func() {
			_ = file_store_factory.Delete(lock_file_spec)
		}()
		defer fd.Close()
		defer zip_writer.Close()

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*600)
		defer cancel()

		err := downloadFlowToZip(ctx, config_obj, client_id, hostname, flow_id, zip_writer)
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

func downloadFlowToZip(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string,
	hostname string,
	flow_id string,
	zip_writer *zip.Writer) error {

	flow_details, err := flows.GetFlowDetails(config_obj, client_id, flow_id)
	if err != nil {
		return err
	}
	file_store_factory := file_store.GetFileStore(config_obj)

	copier := func(upload_name api.FSPathSpec) error {

		reader, err := file_store_factory.ReadFile(upload_name)
		if err != nil {
			return err
		}
		defer reader.Close()

		// Clean the name so it makes a reasonable zip member.
		file_member_name := utils.CleanComponentsForZip(
			upload_name.Components(), client_id, hostname)
		f, err := zip_writer.Create(file_member_name)
		if err != nil {
			return err
		}

		_, err = utils.Copy(ctx, f, reader)
		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.GUIComponent)
			logger.WithFields(logrus.Fields{
				"flow_id":     flow_id,
				"client_id":   client_id,
				"upload_name": upload_name,
			}).Error("Download Flow")
		}
		return err
	}

	flow_path_manager := paths.NewFlowPathManager(client_id, flow_id)

	// Copy the flow's logs.
	err = copier(flow_path_manager.Log())
	if err != nil {
		return err
	}

	// Copy result sets
	for _, artifact_with_results := range flow_details.Context.ArtifactsWithResults {
		// Paths inside the zip file should be friendlier.
		path_manager, err := artifact_paths.NewArtifactPathManager(
			config_obj,
			client_id, flow_details.Context.SessionId,
			artifact_with_results)
		if err != nil {
			return err
		}

		rs_path, err := path_manager.GetPathForWriting()
		if err == nil {
			err = copier(rs_path)
			if err != nil {
				return err
			}
		}

		// Also make a csv file why not?
		zip_file_name := utils.CleanComponentsForZip(rs_path.
			SetType(api.PATH_TYPE_FILESTORE_CSV).
			Components(), client_id, hostname)
		f, err := zip_writer.Create(zip_file_name)
		if err != nil {
			continue
		}

		// File uploads are stored in their own json file.
		reader, err := result_sets.NewResultSetReader(
			file_store_factory, path_manager.Path())
		if err != nil {
			return err
		}
		scope := vql_subsystem.MakeScope()
		csv_writer := csv.GetCSVAppender(scope, f, true /* write_headers */)
		for row := range reader.Rows(ctx) {
			csv_writer.Write(row)
		}
		csv_writer.Close()
	}

	// Get all file uploads if needed
	if flow_details.Context.TotalUploadedFiles == 0 {
		return nil
	}

	// This basically copies the files from the filestore into the
	// zip. We do not need to do any processing - just give the
	// user the files as they are. Users can do their own post
	// processing.

	// File uploads are stored in their own json file.
	reader, err := result_sets.NewResultSetReader(
		file_store_factory, flow_path_manager.UploadMetadata())
	if err != nil {
		return err
	}

	for row := range reader.Rows(ctx) {
		vfs_path_any, pres := row.Get("vfs_path")
		if pres {
			err = copier(vfs_path_any.(api.FSPathSpec))
		}
	}

	return err
}

func createHuntDownloadFile(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	hunt_id string,
	write_json, write_csv bool,
	wait, only_combined bool,
	base_filename string) (api.FSPathSpec, error) {
	if hunt_id == "" {
		return nil, errors.New("Hunt Id should be specified.")
	}

	hunt_path_manager := paths.NewHuntPathManager(hunt_id)
	download_file := hunt_path_manager.GetHuntDownloadsFile(
		only_combined, base_filename)

	logger := logging.GetLogger(config_obj, &logging.GUIComponent)
	logger.WithFields(logrus.Fields{
		"hunt_id":       hunt_id,
		"download_file": download_file,
	}).Info("CreateHuntDownload")

	file_store_factory := file_store.GetFileStore(config_obj)
	lock_file_spec := download_file.SetType(api.PATH_TYPE_FILESTORE_LOCK)
	lock_file, err := file_store_factory.WriteFile(lock_file_spec)
	if err != nil {
		return nil, err
	}
	lock_file.Close()

	fd, err := file_store_factory.WriteFile(download_file)
	if err != nil {
		return nil, err
	}

	err = fd.Truncate()
	if err != nil {
		fd.Close()
		return nil, err
	}

	hunt_details, err := flows.GetHunt(config_obj,
		&api_proto.GetHuntRequest{HuntId: hunt_id})
	if err != nil {
		fd.Close()
		return nil, err
	}

	// Do these first to ensure errors are returned if the zip file
	// is not writable.
	zip_writer := zip.NewWriter(fd)
	f, err := zip_writer.Create("HuntDetails")
	if err != nil {
		zip_writer.Close()
		fd.Close()
		return nil, err
	}

	hunt_details_json, _ := json.ConvertProtoToOrderedDict(hunt_details).MarshalJSON()

	_, err = f.Write(hunt_details_json)
	if err != nil {
		zip_writer.Close()
		fd.Close()
		return nil, err
	}

	wg := sync.WaitGroup{}
	wg.Add(1)

	// Write the bulk of the data asyncronously.
	go func() {
		defer wg.Done()
		defer func() {
			err := file_store_factory.Delete(lock_file_spec)
			if err != nil {
				logger.Error("Failed to bind to remove lock file for %v: %v",
					download_file, err)
			}

		}()
		defer fd.Close()
		defer zip_writer.Close()

		// Allow one hour to write the zip
		ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
		defer cancel()

		// Export aggregate CSV and JSON files for all clients.
		for _, artifact_source := range hunt_details.ArtifactSources {
			artifact, source := paths.SplitFullSourceName(
				artifact_source)

			subscope := scope.Copy()
			subscope.AppendVars(ordereddict.NewDict().
				Set("Artifact", artifact).
				Set("HuntId", hunt_id).
				Set("Source", source))
			defer subscope.Close()

			report_err := func(err error) {
				logging.GetLogger(config_obj, &logging.GUIComponent).
					WithFields(logrus.Fields{
						"artifact": artifact,
						"error":    err,
					}).Error("ExportHuntArtifact")
			}

			query := "SELECT * FROM hunt_results(" +
				"hunt_id=HuntId, artifact=Artifact, " +
				"source=Source)"

			// Write all results to a tmpfile then just
			// copy the tmpfile into the zip.
			json_tmpfile, err := ioutil.TempFile("", "tmp*.json")
			if err != nil {
				report_err(err)
				continue
			}
			defer os.Remove(json_tmpfile.Name())

			csv_tmpfile, err := ioutil.TempFile("", "tmp*.csv")
			if err != nil {
				report_err(err)
				continue
			}
			defer os.Remove(csv_tmpfile.Name())

			err = StoreVQLAsCSVAndJsonFile(ctx, config_obj,
				subscope, query, write_csv, write_json,
				csv_tmpfile, json_tmpfile)
			if err != nil {
				report_err(err)
				continue
			}

			// Make sure the files are closed so we can
			// open them for reading next.
			csv_tmpfile.Close()
			json_tmpfile.Close()

			copier := func(name string, output_name string) error {
				reader, err := os.Open(name)
				if err != nil {
					return err
				}
				defer reader.Close()

				// Clean the name so it makes a reasonable zip member.
				f, err := zip_writer.Create(
					utils.CleanComponentsForZip(
						utils.SplitComponents(
							output_name), "", ""))
				if err != nil {
					return err
				}

				_, err = utils.Copy(ctx, f, reader)
				return err
			}

			if write_csv {
				err = copier(csv_tmpfile.Name(), "All "+
					path.Join(artifact, source)+".csv")
				if err != nil {
					report_err(err)
					continue
				}
			}

			if write_json {
				err = copier(json_tmpfile.Name(), "All "+
					path.Join(artifact, source)+".json")
				if err != nil {
					report_err(err)
				}
			}
		}

		// If the user only asked for combined results do not
		// export specific flow.
		if only_combined {
			return
		}

		subscope := scope.Copy()
		subscope.AppendVars(ordereddict.NewDict().
			Set("HuntId", hunt_id))
		defer subscope.Close()

		query := "SELECT Flow.session_id AS FlowId, ClientId " +
			"FROM hunt_flows(hunt_id=HuntId)"
		vql, _ := vfilter.Parse(query)

		query_log := actions.QueryLog.AddQuery(query)
		defer query_log.Close()

		for row := range vql.Eval(ctx, subscope) {
			flow_id := vql_subsystem.GetStringFromRow(scope, row, "FlowId")
			client_id := vql_subsystem.GetStringFromRow(scope, row, "ClientId")
			if flow_id == "" || client_id == "" {
				continue
			}

			hostname := services.GetHostname(client_id)
			err := downloadFlowToZip(
				ctx, config_obj, client_id, hostname, flow_id, zip_writer)
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

func StoreVQLAsCSVAndJsonFile(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	query string,
	write_csv bool,
	write_json bool,
	csv_fd io.Writer,
	json_fd io.Writer) error {

	query_log := actions.QueryLog.AddQuery(query)
	defer query_log.Close()
	vql, err := vfilter.Parse(query)
	if err != nil {
		return err
	}

	csv_writer := csv.GetCSVAppender(scope, csv_fd, true /* write_headers */)
	defer csv_writer.Close()

	sub_ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for row := range vql.Eval(sub_ctx, scope) {
		if write_csv {
			csv_writer.Write(row)
		}

		if write_json {
			serialized, err := json.Marshal(row)
			if err != nil {
				continue
			}
			_, err = json_fd.Write(serialized)
			if err != nil {
				return errors.WithStack(err)
			}
			_, err = json_fd.Write([]byte("\n"))
			if err != nil {
				return errors.WithStack(err)
			}
		}
	}

	return nil
}

func init() {
	vql_subsystem.RegisterFunction(&CreateHuntDownload{})
	vql_subsystem.RegisterFunction(&CreateFlowDownload{})
}
