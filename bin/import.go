package main

import (
	"archive/zip"
	"fmt"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/result_sets"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	artifact_paths "www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	// Command line interface for VQL commands.
	import_cmd    = app.Command("import", "Import an offline collection to a client")
	import_client = import_cmd.Flag("client_id", "The client id to import the client for.").
			Required().String()

	import_file = import_cmd.Arg("filename", "The offline zip file to import").
			Required().String()
)

func doImport() {
	config_obj, err := DefaultConfigLoader.
		WithRequiredFrontend().
		WithRequiredUser().
		WithRequiredLogging().LoadAndValidate()
	kingpin.FatalIfError(err, "Unable to load config file")

	ctx, cancel := install_sig_handler()
	defer cancel()
	_ = ctx

	sm, err := startEssentialServices(config_obj)
	kingpin.FatalIfError(err, "Load Config ")
	defer sm.Close()

	db, err := datastore.GetDB(config_obj)
	kingpin.FatalIfError(err, "Saving file ")

	file_store_factory := file_store.GetFileStore(config_obj)

	manager, err := services.GetRepositoryManager()
	kingpin.FatalIfError(err, "Load Config ")

	repository, err := manager.GetGlobalRepository(config_obj)
	kingpin.FatalIfError(err, "Load Config ")

	zipfile, err := zip.OpenReader(*import_file)
	kingpin.FatalIfError(err, "Loading collection file ")
	defer zipfile.Close()

	artifacts := make(map[string]bool)
	uploads := []string{}

	// Create a new flow and path manager for it.
	client_id := *import_client
	flow_id := launcher.NewFlowId(client_id)
	path_manager := paths.NewFlowPathManager(client_id, flow_id)
	new_flow := &flows_proto.ArtifactCollectorContext{
		SessionId:  flow_id,
		ClientId:   client_id,
		Request:    &flows_proto.ArtifactCollectorArgs{},
		CreateTime: uint64(time.Now().UnixNano() / 1000),
		State:      flows_proto.ArtifactCollectorContext_FINISHED,
	}

	uploaded_files_result_set, err := result_sets.NewResultSetWriter(
		file_store_factory, path_manager.UploadMetadata(),
		nil, true /* truncate */)
	kingpin.FatalIfError(err, "Loading collection file ")
	defer uploaded_files_result_set.Close()

	log_result_set, err := result_sets.NewResultSetWriter(
		file_store_factory, path_manager.Log(),
		nil, true /* truncate */)
	kingpin.FatalIfError(err, "Loading collection file ")
	defer log_result_set.Close()

	log := func(format string, args ...interface{}) {
		now := time.Now().UTC()

		log_result_set.Write(ordereddict.NewDict().
			Set("Timestamp", fmt.Sprintf("%v", now)).
			Set("time", time.Unix(int64(now.UnixNano())/1000000, 0).String()).
			Set("message", fmt.Sprintf(format, args...)))

		fmt.Printf(format+"\n", args...)
	}

	log("Importing zip file %v into client id %v", *import_file, client_id)

	for _, file := range zipfile.File {
		log("Filename %v", file.Name)

		// Files can be either an artifact or an upload
		artifact_name := strings.TrimSuffix(file.Name, ".json")
		artifact, pres := repository.Get(config_obj, artifact_name)
		if pres {
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
					config_obj, client_id, flow_id, artifact_name)

				rs_writer, err := result_sets.NewResultSetWriter(
					file_store_factory, artifact_path_manager,
					nil, true /* truncate */)
				if err != nil {
					log("Error copying %v", err)
					return
				}
				defer rs_writer.Close()

				// Now copy the rows from the zip to the filestore.
				for row := range utils.ReadJsonFromFile(ctx, fd) {
					new_flow.TotalCollectedRows++
					rs_writer.Write(row)
				}
			}()
		} else {
			uploads = append(uploads, file.Name)
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

	for k, _ := range artifacts {
		new_flow.Request.Artifacts = append(new_flow.Request.Artifacts, k)
	}

	err = db.SetSubject(config_obj, path_manager.Path(), new_flow)
	kingpin.FatalIfError(err, "Saving new collection ")
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case "import":
			doImport()
		default:
			return false
		}
		return true
	})
}
