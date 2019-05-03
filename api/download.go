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

	"github.com/golang/protobuf/jsonpb"
	"github.com/gorilla/schema"
	errors "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/flows"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

func returnError(w http.ResponseWriter, code int, message string) {
	w.WriteHeader(code)
	w.Write([]byte(message))
}

func downloadFlowToZip(
	config_obj *api_proto.Config,
	client_id string,
	flow_id string,
	zip_writer *zip.Writer) error {

	flow_details, err := flows.GetFlowDetails(config_obj, client_id, flow_id)
	if err != nil {
		return err
	}

	// This basically copies the CSV files from the
	// filestore into the zip. We do not need to do any
	// processing - just give the user the files as they
	// are. Users can do their own post processing.
	file_store_factory := file_store.GetFileStore(config_obj)
	for _, artifact := range flow_details.Context.Artifacts {
		file_path := path.Join(
			"clients", client_id,
			"artifacts", artifact,
			path.Base(flow_id)+".csv")

		fd, err := file_store_factory.ReadFile(file_path)
		if err != nil {
			continue
		}

		zh, err := zip_writer.Create(file_path)
		if err != nil {
			continue
		}

		_, err = io.Copy(zh, fd)
		if err != nil {
			return err
		}
	}

	// Get all file uploads
	for _, upload_name := range flow_details.Context.UploadedFiles {
		reader, err := file_store_factory.ReadFile(upload_name)
		if err != nil {
			continue
		}

		f, err := zip_writer.Create(upload_name)
		if err != nil {
			continue
		}

		_, err = io.Copy(f, reader)
		if err != nil {
			continue
		}
	}
	return err
}

// URL format: /api/v1/download/<client_id>/<flow_id>
func flowResultDownloadHandler(
	config_obj *api_proto.Config) http.Handler {
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
		userinfo := logging.GetUserInfo(r.Context(), config_obj)

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

		downloadFlowToZip(config_obj, client_id, flow_id, zip_writer)
	})
}

// URL format: /api/v1/DownloadHuntResults
func huntResultDownloadHandler(
	config_obj *api_proto.Config) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hunt_ids, pres := r.URL.Query()["hunt_id"]
		if !pres || len(hunt_ids) == 0 {
			returnError(w, 404, "Hunt id should be specified.")
			return
		}
		hunt_id := path.Base(hunt_ids[0])

		hunt_details, err := flows.GetHunt(
			config_obj,
			&api_proto.GetHuntRequest{HuntId: hunt_id})
		if err != nil {
			returnError(w, 404, err.Error())
			return
		}
		flows.FindCollectedArtifacts(hunt_details)

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
		userinfo := logging.GetUserInfo(r.Context(), config_obj)
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
		for _, artifact := range hunt_details.Artifacts {
			query := "SELECT * FROM hunt_results(" +
				"hunt_id=HuntId, artifact=Artifact, brief=true)"
			env := vfilter.NewDict().
				Set("Artifact", artifact).
				Set("HuntId", hunt_id)

			f, err := zip_writer.Create("All " + artifact + ".csv")
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
				config_obj,
				client_id,
				flow_id,
				zip_writer)
			if err != nil {
				return
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

func openBuiltInArtifact(config_obj *api_proto.Config, vfs_path string) (
	file_store.ReadSeekCloser, error) {
	repository, err := artifacts.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, err
	}

	artifact_path := path.Join("/", strings.TrimPrefix(
		vfs_path, constants.BUILTIN_ARTIFACT_DEFINITION))

	for _, artifact_obj := range repository.Data {
		artifact_obj_path := artifacts.NameToPath(artifact_obj.Name)
		if artifact_obj_path == artifact_path {
			return utils.DataReadSeekCloser{
				strings.NewReader(artifact_obj.Raw),
			}, nil
		}
	}

	return nil, errors.New("not found")
}

func filestorePathForVFSPath(
	config_obj *api_proto.Config,
	client_id string,
	vfs_path string) string {
	vfs_path = path.Join("/", vfs_path)

	// monitoring and artifacts vfs folders are in the client's
	// space.
	if strings.HasPrefix(vfs_path, "/monitoring/") ||
		strings.HasPrefix(vfs_path, "/flows/") ||
		strings.HasPrefix(vfs_path, "/artifacts/") {
		return path.Join(
			"clients", client_id, vfs_path)
	}

	// These VFS directories are mapped directly to the root of
	// the filestore regardless of the client id.
	if strings.HasPrefix(
		vfs_path, constants.ARTIFACT_DEFINITION) ||
		strings.HasPrefix(vfs_path, "/exported_files/") ||
		strings.HasPrefix(vfs_path, "/server_artifacts/") ||
		strings.HasPrefix(vfs_path, "/hunts/") {
		return vfs_path
	}

	// Other folders live inside the client's vfs_files subdir.
	return path.Join(
		"clients", client_id,
		"vfs_files", vfs_path)
}

func getFileForVFSPath(
	config_obj *api_proto.Config,
	client_id string,
	vfs_path string) (
	file_store.ReadSeekCloser, error) {
	vfs_path = path.Clean(vfs_path)

	if strings.HasPrefix(vfs_path,
		constants.BUILTIN_ARTIFACT_DEFINITION) {
		return openBuiltInArtifact(config_obj, vfs_path)

	}

	filestore_path := filestorePathForVFSPath(config_obj, client_id, vfs_path)
	return file_store.GetFileStore(config_obj).ReadFile(filestore_path)
}

// URL format: /api/v1/DownloadVFSFile
func vfsFileDownloadHandler(
	config_obj *api_proto.Config) http.Handler {
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
	config_obj *api_proto.Config) http.Handler {
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
		userinfo := logging.GetUserInfo(r.Context(), config_obj)

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
			func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return nil
				}

				fd, err := file_store_factory.ReadFile(path)
				if err != nil {
					logger.Warn("Cant open %s: %v", path, err)
					return nil
				}

				zh, err := zip_writer.Create(path)
				if err != nil {
					logger.Warn("Cant create zip %s: %v", path, err)
					return nil
				}

				_, err = io.Copy(zh, fd)
				if err != nil {
					logger.Warn("Cant copy %s", path)
					return nil
				}
				return nil
			})
	})
}

func vfsGetBuffer(
	config_obj *api_proto.Config,
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
