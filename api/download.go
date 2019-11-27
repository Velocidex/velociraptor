/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
// Implement downloads. For now we do not use gRPC for this but
// implement it directly in the API.
package api

import (
	"archive/zip"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/golang/protobuf/jsonpb"
	"github.com/gorilla/schema"
	errors "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/flows"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

func returnError(w http.ResponseWriter, code int, message string) {
	w.WriteHeader(code)
	w.Write([]byte(message))
}

func downloadFlowToZip(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string,
	flow_id string,
	zip_writer *zip.Writer) error {

	flow_details, err := flows.GetFlowDetails(config_obj, client_id, flow_id)
	if err != nil {
		return err
	}
	file_store_factory := file_store.GetFileStore(config_obj)

	copier := func(upload_name string) error {
		reader, err := file_store_factory.ReadFile(upload_name)
		if err != nil {
			return err
		}
		defer reader.Close()

		// Clean the name so it makes a reasonable zip member.
		upload_name = path.Clean(strings.Replace(
			upload_name, "\\", "/", -1))

		// Zip files should not have absolute paths
		upload_name = strings.TrimLeft(upload_name, "/")
		f, err := zip_writer.Create(upload_name)
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

	// Copy the flow's logs.
	copier(path.Join(flow_details.Context.Urn, "logs"))

	// Copy CSV files
	for _, artifacts_with_results := range flow_details.Context.ArtifactsWithResults {
		artifact_name, source := artifacts.SplitFullSourceName(artifacts_with_results)
		csv_path := artifacts.GetCSVPath(
			flow_details.Context.Request.ClientId, "*",
			flow_details.Context.SessionId,
			artifact_name, source, artifacts.MODE_CLIENT)
		copier(csv_path)
	}

	// Get all file uploads
	if flow_details.Context.TotalUploadedFiles == 0 {
		return nil
	}

	// This basically copies the CSV files from the
	// filestore into the zip. We do not need to do any
	// processing - just give the user the files as they
	// are. Users can do their own post processing.

	// File uploads are stored in their own CSV file.
	file_path := artifacts.GetUploadsMetadata(client_id, flow_id)
	fd, err := file_store_factory.ReadFile(file_path)
	if err != nil {
		return err
	}
	defer fd.Close()

	for row := range csv.GetCSVReader(fd) {
		vfs_path_any, pres := row.Get("vfs_path")
		if pres {
			err = copier(vfs_path_any.(string))
		}
	}

	return err
}

func createDownloadFile(config_obj *config_proto.Config,
	flow_id string, client_id string) error {
	if client_id == "" || flow_id == "" {
		return errors.New("Client Id and Flow Id should be specified.")
	}

	download_file := artifacts.GetDownloadsFile(client_id, flow_id)

	logger := logging.GetLogger(config_obj, &logging.GUIComponent)
	logger.WithFields(logrus.Fields{
		"flow_id":       flow_id,
		"client_id":     client_id,
		"download_file": download_file,
	}).Error("CreateDownload")

	file_store_factory := file_store.GetFileStore(config_obj)
	fd, err := file_store_factory.WriteFile(download_file)
	if err != nil {
		return err
	}

	lock_file, err := file_store_factory.WriteFile(download_file + ".lock")
	if err != nil {
		return err
	}
	lock_file.Close()

	flow_details, err := flows.GetFlowDetails(config_obj, client_id, flow_id)
	if err != nil {
		return err
	}

	marshaler := &jsonpb.Marshaler{Indent: " "}
	flow_details_json, err := marshaler.MarshalToString(flow_details)
	if err != nil {
		fd.Close()
		return err
	}

	// Do these first to ensure errors are returned if the zip file
	// is not writable.
	zip_writer := zip.NewWriter(fd)
	f, err := zip_writer.Create("FlowDetails")
	if err != nil {
		fd.Close()
		return err
	}

	_, err = f.Write([]byte(flow_details_json))
	if err != nil {
		zip_writer.Close()
		fd.Close()
		return err
	}

	// Write the bulk of the data asyncronously.
	go func() {
		defer file_store_factory.Delete(download_file + ".lock")
		defer fd.Close()
		defer zip_writer.Close()

		downloadFlowToZip(context.Background(),
			config_obj, client_id, flow_id, zip_writer)
	}()

	return nil
}

// DEPRECATED:
// URL format: /api/v1/download/<client_id>/<flow_id>
func flowResultDownloadHandler(
	config_obj *config_proto.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		components := strings.Split(r.URL.Path, "/")
		if len(components) < 2 {
			returnError(w, 404, "Flow id should be specified.")
			return
		}
		flow_id := components[len(components)-1]
		client_id := components[len(components)-2]
		flow_details, err := flows.GetFlowDetails(config_obj, client_id, flow_id)
		if err != nil {
			returnError(w, 404, err.Error())
			return
		}

		// TODO: ACL checks.
		if r.Method == "HEAD" {
			returnError(w, 200, "Ok")
			return
		}

		// From here on we already sent the headers and we can
		// not really report an error to the client.
		w.Header().Set("Content-Disposition", "attachment; filename="+
			url.PathEscape(flow_id+".zip"))
		w.Header().Set("Content-Type", "binary/octet-stream")
		w.WriteHeader(200)

		// Log an audit event.
		userinfo := GetUserInfo(r.Context(), config_obj)

		// This should never happen!
		if userinfo.Name == "" {
			panic("Unauthenticated access.")
		}

		logging.GetLogger(config_obj, &logging.Audit).
			WithFields(logrus.Fields{
				"user":      userinfo.Name,
				"flow_id":   flow_id,
				"client_id": client_id,
				"remote":    r.RemoteAddr,
			}).Info("DownloadFlowResults")

		marshaler := &jsonpb.Marshaler{Indent: " "}
		flow_details_json, err := marshaler.MarshalToString(flow_details)
		if err != nil {
			return
		}

		zip_writer := zip.NewWriter(w)
		defer zip_writer.Close()

		f, err := zip_writer.Create("FlowDetails")
		if err != nil {
			return
		}

		_, err = f.Write([]byte(flow_details_json))
		if err != nil {
			return
		}

		downloadFlowToZip(r.Context(),
			config_obj, client_id, flow_id, zip_writer)
	})
}

func createHuntDownloadFile(
	config_obj *config_proto.Config, hunt_id string) error {
	if hunt_id == "" {
		return errors.New("Hunt Id should be specified.")
	}
	download_file := artifacts.GetHuntDownloadsFile(hunt_id)

	logger := logging.GetLogger(config_obj, &logging.GUIComponent)
	logger.WithFields(logrus.Fields{
		"hunt_id":       hunt_id,
		"download_file": download_file,
	}).Info("CreateHuntDownload")

	file_store_factory := file_store.GetFileStore(config_obj)

	lock_file, err := file_store_factory.WriteFile(download_file + ".lock")
	if err != nil {
		return err
	}
	lock_file.Close()

	fd, err := file_store_factory.WriteFile(download_file)
	if err != nil {
		return err
	}

	hunt_details, err := flows.GetHunt(config_obj,
		&api_proto.GetHuntRequest{HuntId: hunt_id})
	if err != nil {
		return err
	}

	marshaler := &jsonpb.Marshaler{Indent: " "}
	hunt_details_json, err := marshaler.MarshalToString(hunt_details)
	if err != nil {
		fd.Close()
		return err
	}

	// Do these first to ensure errors are returned if the zip file
	// is not writable.
	zip_writer := zip.NewWriter(fd)
	f, err := zip_writer.Create("HuntDetails")
	if err != nil {
		fd.Close()
		return err
	}

	_, err = f.Write([]byte(hunt_details_json))
	if err != nil {
		zip_writer.Close()
		fd.Close()
		return err
	}

	// Write the bulk of the data asyncronously.
	go func() {
		defer file_store_factory.Delete(download_file + ".lock")
		defer fd.Close()
		defer zip_writer.Close()

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*600)
		defer cancel()

		// Export aggregate CSV files for all clients.
		for _, artifact_source := range hunt_details.ArtifactSources {
			artifact, source := artifacts.SplitFullSourceName(
				artifact_source)

			query := "SELECT * FROM hunt_results(" +
				"hunt_id=HuntId, artifact=Artifact, " +
				"source=Source, brief=true)"
			env := ordereddict.NewDict().
				Set("Artifact", artifact).
				Set("HuntId", hunt_id).
				Set("Source", source)

			f, err := zip_writer.Create("All " +
				path.Join(artifact, source) + ".csv")
			if err != nil {
				continue
			}

			err = StoreVQLAsCSVFile(ctx, config_obj, env, query, f)
			if err != nil {
				logging.GetLogger(config_obj, &logging.GUIComponent).
					WithFields(logrus.Fields{
						"artifact": artifact,
						"error":    err,
					}).Error("ExportHuntArtifact")
			}
		}

		file_store_factory := file_store.GetFileStore(config_obj)
		file_path := path.Join("hunts", hunt_id+".csv")
		fd, err := file_store_factory.ReadFile(file_path)
		if err != nil {
			return
		}
		defer fd.Close()

		for row := range csv.GetCSVReader(fd) {
			flow_id_any, _ := row.Get("FlowId")
			flow_id, ok := flow_id_any.(string)
			if !ok {
				continue
			}
			client_id_any, _ := row.Get("ClientId")
			client_id, ok := client_id_any.(string)
			if !ok {
				continue
			}

			err := downloadFlowToZip(
				ctx, config_obj, client_id, flow_id, zip_writer)
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

	return nil
}

// To be deprecated.
// URL format: /api/v1/DownloadHuntResults
func huntResultDownloadHandler(
	config_obj *config_proto.Config) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hunt_ids, pres := r.URL.Query()["hunt_id"]
		if !pres || len(hunt_ids) == 0 {
			returnError(w, 404, "Hunt id should be specified.")
			return
		}
		hunt_id := hunt_ids[0]

		hunt_details, err := flows.GetHunt(
			config_obj,
			&api_proto.GetHuntRequest{HuntId: hunt_id})
		if err != nil {
			returnError(w, 404, err.Error())
			return
		}

		// TODO: ACL checks.
		if r.Method == "HEAD" {
			returnError(w, 200, "Ok")
			return
		}

		// From here on we sent the headers and we can not
		// really report an error to the client.
		w.Header().Set("Content-Disposition", "attachment; filename="+
			url.PathEscape(hunt_id+".zip"))
		w.Header().Set("Content-Type", "binary/octet-stream")
		w.WriteHeader(200)

		// Log an audit event.
		userinfo := GetUserInfo(r.Context(), config_obj)
		logging.GetLogger(config_obj, &logging.Audit).
			WithFields(logrus.Fields{
				"user":    userinfo.Name,
				"hunt_id": hunt_id,
				"remote":  r.RemoteAddr,
			}).Info("DownloadHuntResults")

		// This should never happen!
		if userinfo.Name == "" {
			panic("Unauthenticated access.")
		}

		marshaler := &jsonpb.Marshaler{Indent: " "}
		hunt_details_json, err := marshaler.MarshalToString(hunt_details)
		if err != nil {
			return
		}

		zip_writer := zip.NewWriter(w)
		defer zip_writer.Close()

		f, err := zip_writer.Create("HuntDetails")
		if err != nil {
			return
		}

		_, err = f.Write([]byte(hunt_details_json))
		if err != nil {
			return
		}

		// Export aggregate CSV files for all clients.
		for _, artifact_source := range hunt_details.ArtifactSources {
			artifact, source := artifacts.SplitFullSourceName(
				artifact_source)

			query := "SELECT * FROM hunt_results(" +
				"hunt_id=HuntId, artifact=Artifact, " +
				"source=Source, brief=true)"
			env := ordereddict.NewDict().
				Set("Artifact", artifact).
				Set("HuntId", hunt_id).
				Set("Source", source)

			f, err := zip_writer.Create("All " +
				path.Join(artifact, source) + ".csv")
			if err != nil {
				continue
			}

			err = StoreVQLAsCSVFile(r.Context(), config_obj,
				env, query, f)
			if err != nil {
				logging.GetLogger(config_obj, &logging.Audit).
					WithFields(logrus.Fields{
						"artifact": artifact,
					}).Info("ExportHuntArtifact")
			}
		}

		file_store_factory := file_store.GetFileStore(config_obj)
		file_path := path.Join("hunts", hunt_id+".csv")
		fd, err := file_store_factory.ReadFile(file_path)
		if err != nil {
			return
		}
		defer fd.Close()

		for row := range csv.GetCSVReader(fd) {
			flow_id_any, _ := row.Get("FlowId")
			flow_id, ok := flow_id_any.(string)
			if !ok {
				continue
			}
			client_id_any, _ := row.Get("ClientId")
			client_id, ok := client_id_any.(string)
			if !ok {
				continue
			}

			err := downloadFlowToZip(
				r.Context(),
				config_obj,
				client_id,
				flow_id,
				zip_writer)
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
	})
}

type vfsFileDownloadRequest struct {
	ClientId string `schema:"client_id"`
	VfsPath  string `schema:"vfs_path,required"`
	Offset   int64  `schema:"offset"`
	Length   int    `schema:"length"`
	Encoding string `schema:"encoding"`
}

func filestorePathForVFSPath(
	config_obj *config_proto.Config,
	client_id string,
	vfs_path string) string {
	vfs_path = path.Join("/", vfs_path)

	// monitoring and artifacts vfs folders are in the client's
	// space.
	if strings.HasPrefix(vfs_path, "/monitoring/") ||
		strings.HasPrefix(vfs_path, "/collections/") ||
		strings.HasPrefix(vfs_path, "/artifacts/") {
		return path.Join(
			"clients", client_id, vfs_path)
	}

	// These VFS directories are mapped directly to the root of
	// the filestore regardless of the client id.
	if strings.HasPrefix(vfs_path, "/server_artifacts/") ||
		strings.HasPrefix(vfs_path, "/hunts/") ||
		strings.HasPrefix(vfs_path, "/clients/") {
		return utils.Normalize_windows_path(vfs_path)
	}

	// Other folders live inside the client's vfs_files subdir.
	return path.Join(
		"clients", client_id,
		"vfs_files", vfs_path)
}

func getFileForVFSPath(
	config_obj *config_proto.Config,
	client_id string,
	vfs_path string) (
	file_store.ReadSeekCloser, error) {
	vfs_path = path.Clean(vfs_path)

	filestore_path := filestorePathForVFSPath(config_obj, client_id, vfs_path)
	return file_store.GetFileStore(config_obj).ReadFile(filestore_path)
}

// URL format: /api/v1/DownloadVFSFile
func vfsFileDownloadHandler(
	config_obj *config_proto.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := vfsFileDownloadRequest{}
		decoder := schema.NewDecoder()
		err := decoder.Decode(&request, r.URL.Query())
		if err != nil {
			returnError(w, 404, err.Error())
			return
		}

		file, err := getFileForVFSPath(
			config_obj, request.ClientId, request.VfsPath)
		if err != nil {
			returnError(w, 404, err.Error())
			return
		}
		defer file.Close()

		if r.Method == "HEAD" {
			returnError(w, 200, "Ok")
			return
		}

		file.Seek(request.Offset, 0)

		// From here on we sent the headers and we can not
		// really report an error to the client.
		filename := strings.Replace(path.Dir(request.VfsPath),
			"\"", "_", -1)
		w.Header().Set("Content-Disposition", "attachment; filename="+
			url.PathEscape(filename))
		w.Header().Set("Content-Type", "binary/octet-stream")
		w.WriteHeader(200)

		length_sent := 0
		buf := make([]byte, 64*1024)
		for {
			n, _ := file.Read(buf)
			if n > 0 {
				if request.Length != 0 {
					length_to_send := request.Length - length_sent
					if n > length_to_send {
						n = length_to_send
					}
				}
				_, err := w.Write(buf[:n])
				if err != nil {
					return
				}
				length_sent += n

			} else {
				return
			}
		}
	})
}

// URL format: /api/v1/DownloadVFSFolder
func vfsFolderDownloadHandler(
	config_obj *config_proto.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := vfsFileDownloadRequest{}
		decoder := schema.NewDecoder()
		err := decoder.Decode(&request, r.URL.Query())
		if err != nil {
			returnError(w, 404, err.Error())
			return
		}

		if r.Method == "HEAD" {
			returnError(w, 200, "Ok")
			return
		}

		// From here on we already sent the headers and we can
		// not really report an error to the client.
		w.Header().Set("Content-Disposition", "attachment; filename="+
			url.PathEscape(request.VfsPath+".zip"))
		w.Header().Set("Content-Type", "binary/octet-stream")
		w.WriteHeader(200)

		// Log an audit event.
		userinfo := GetUserInfo(r.Context(), config_obj)

		// This should never happen!
		if userinfo.Name == "" {
			panic("Unauthenticated access.")
		}

		logger := logging.GetLogger(config_obj, &logging.Audit)
		logger.WithFields(logrus.Fields{
			"user":      userinfo.Name,
			"vfs_path":  request.VfsPath,
			"client_id": request.ClientId,
			"remote":    r.RemoteAddr,
		}).Info("DownloadVFSPath")

		zip_writer := zip.NewWriter(w)
		defer zip_writer.Close()

		file_store_factory := file_store.GetFileStore(config_obj)
		file_store_factory.Walk(filestorePathForVFSPath(
			config_obj, request.ClientId, request.VfsPath),
			func(path_name string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return nil
				}

				fd, err := file_store_factory.ReadFile(path_name)
				if err != nil {
					logger.Warn("Cant open %s: %v", path_name, err)
					return nil
				}

				path_name = path.Clean(strings.Replace(
					path_name, "\\", "/", -1))

				path_name = strings.TrimLeft(path_name, "/")
				zh, err := zip_writer.Create(path_name)
				if err != nil {
					logger.Warn("Cant create zip %s: %v", path_name, err)
					return nil
				}

				_, err = utils.Copy(r.Context(), zh, fd)
				if err != nil {
					logger.Warn("Cant copy %s", path_name)
					return nil
				}
				return nil
			})
	})
}

func vfsGetBuffer(
	config_obj *config_proto.Config,
	client_id string, vfs_path string, offset uint64, length uint32) (
	*api_proto.VFSFileBuffer, error) {

	file, err := getFileForVFSPath(
		config_obj, client_id, vfs_path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	file.Seek(int64(offset), 0)

	result := &api_proto.VFSFileBuffer{
		Data: make([]byte, length),
	}

	n, err := io.ReadAtLeast(file, result.Data, len(result.Data))
	if err != nil && errors.Cause(err) != io.EOF &&
		errors.Cause(err) != io.ErrUnexpectedEOF {
		return nil, err
	}

	result.Data = result.Data[:n]

	return result, nil
}
