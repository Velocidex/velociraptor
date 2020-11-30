package main

import (
	"archive/zip"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/api"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/result_sets"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	artifact_paths "www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	// Command line interface for VQL commands.
	import_cmd    = app.Command("import", "Import an offline collection to a client")
	import_client = import_cmd.Flag("client_id", "The client id to import the client for.").
			String()

	import_hostname = import_cmd.Flag("hostname", "The hostname to import the client for.").
			String()

	import_create_host = import_cmd.Flag("create", "Create a client id record if needed.").
				Bool()

	import_file = import_cmd.Arg("filename", "The offline zip file to import").
			Required().String()
)

// Generate a new client id
func NewClientId() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	dst := make([]byte, hex.EncodedLen(8))
	hex.Encode(dst, buf)
	return "C." + string(dst)
}

// Retrieve the client id from the hostname.
func getClientId(config_obj *config_proto.Config,
	hostname string) string {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return ""
	}

	for _, client_id := range db.SearchClients(
		config_obj, constants.CLIENT_INDEX_URN,
		hostname, "", 0, 1, datastore.SORT_UP) {
		return client_id
	}
	return ""
}

// Create a new client record
func makeNewClient(
	config_obj *config_proto.Config,
	hostname string) (string, error) {
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

func doImport() {
	if *import_client == "" && *import_hostname == "" {
		kingpin.Fatalf("One of client_id or hostname should be specified.")
	}

	config_obj, err := DefaultConfigLoader.
		WithRequiredFrontend().
		WithRequiredUser().
		WithRequiredLogging().LoadAndValidate()
	kingpin.FatalIfError(err, "Unable to load config file")

	ctx, cancel := install_sig_handler()
	defer cancel()

	sm, err := startEssentialServices(config_obj)
	kingpin.FatalIfError(err, "Load Config ")
	defer sm.Close()

	db, err := datastore.GetDB(config_obj)
	kingpin.FatalIfError(err, "Saving file ")

	client_id := *import_client
	hostname := *import_hostname
	if hostname != "" {
		client_id = getClientId(config_obj, hostname)
		if client_id == "" {
			if !*import_create_host {
				kingpin.Fatalf("Hostname %v not known, you can "+
					"create one with the --create flag ", hostname)
			}
			client_id, err = makeNewClient(config_obj, hostname)
			kingpin.FatalIfError(err, "Creating new client")
		}
	}

	api_client, err := api.GetApiClient(config_obj, nil, client_id, false /* detailed */)
	kingpin.FatalIfError(err, "Client ID Not known ")

	if api_client.AgentInformation.Name == "" {
		kingpin.Fatalf("Client ID not known")
	}

	file_store_factory := file_store.GetFileStore(config_obj)

	repository, err := getRepository(config_obj)
	kingpin.FatalIfError(err, "Load Config ")

	// Open the zip file we are importing.
	zipfile, err := zip.OpenReader(*import_file)
	kingpin.FatalIfError(err, "Loading collection file ")
	defer zipfile.Close()

	// Keep track of all the artifacts in the zip file.
	artifacts := make(map[string]bool)
	uploads := []string{}

	// Create a new flow and path manager for it.
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

	// A log function that stores messages in the flow log as well
	// as print them to the screen.
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

	// Copy all unique artifacts to the request struct - this will
	// go into the flow context.
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
