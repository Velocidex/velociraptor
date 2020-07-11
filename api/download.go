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

// Implement downloads. We do not use gRPC for this but implement it
// directly in the API.
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
	"github.com/tink-ab/tempfile"
	context "golang.org/x/net/context"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/flows"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

func returnError(w http.ResponseWriter, code int, message string) {
	w.WriteHeader(code)
	w.Write([]byte(message))
}

func cleanPathForZip(filename string) string {
	filename = path.Clean(strings.Replace(
		filename, "\\", "/", -1))

	// Zip files should not have absolute paths
	filename = strings.TrimLeft(filename, "/")
	return filename
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
		f, err := zip_writer.Create(cleanPathForZip(upload_name))
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
	copier(flow_path_manager.Log().Path())

	// Copy result sets
	for _, artifact_with_results := range flow_details.Context.ArtifactsWithResults {
		path_manager := result_sets.NewArtifactPathManager(config_obj,
			flow_details.Context.Request.ClientId,
			flow_details.Context.SessionId, artifact_with_results)
		rs_path, err := path_manager.GetPathForWriting()
		if err == nil {
			copier(rs_path)
		}

		// Also make a csv file why not?
		f, err := zip_writer.Create(cleanPathForZip(
			strings.TrimSuffix(rs_path, ".json") + ".csv"))
		if err != nil {
			continue
		}

		// File uploads are stored in their own json file.
		row_chan, err := file_store.GetTimeRange(
			ctx, config_obj, path_manager, 0, 0)
		if err != nil {
			return err
		}
		scope := vql_subsystem.MakeScope()
		csv_writer := csv.GetCSVAppender(scope, f, true /* write_headers */)
		for row := range row_chan {
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
	row_chan, err := file_store.GetTimeRange(
		ctx, config_obj, flow_path_manager.UploadMetadata(), 0, 0)
	if err != nil {
		return err
	}

	for row := range row_chan {
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

	flow_path_manager := paths.NewFlowPathManager(client_id, flow_id)
	download_file := flow_path_manager.GetDownloadsFile().Path()

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

	err = fd.Truncate()
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

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*600)
		defer cancel()

		downloadFlowToZip(ctx, config_obj, client_id, flow_id, zip_writer)
	}()

	return nil
}

func createHuntDownloadFile(
	config_obj *config_proto.Config,
	principal string,
	hunt_id string) error {
	if hunt_id == "" {
		return errors.New("Hunt Id should be specified.")
	}
	download_file := paths.GetHuntDownloadsFile(hunt_id)

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

	err = fd.Truncate()
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
		zip_writer.Close()
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

		// Allow one hour to write the zip
		ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
		defer cancel()

		// Export aggregate CSV and JSON files for all clients.
		for _, artifact_source := range hunt_details.ArtifactSources {
			artifact, source := paths.SplitFullSourceName(
				artifact_source)

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
			env := ordereddict.NewDict().
				Set("Artifact", artifact).
				Set("HuntId", hunt_id).
				Set("Source", source)

			// Write all results to a tmpfile then just
			// copy the tmpfile into the zip.
			json_tmpfile, err := tempfile.TempFile("", "tmp", ".json")
			if err != nil {
				report_err(err)
				continue
			}
			defer os.Remove(json_tmpfile.Name())

			csv_tmpfile, err := tempfile.TempFile("", "tmp", ".csv")
			if err != nil {
				report_err(err)
				continue
			}
			defer os.Remove(csv_tmpfile.Name())

			err = StoreVQLAsCSVAndJsonFile(ctx, config_obj,
				principal, env, query,
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
				f, err := zip_writer.Create(cleanPathForZip(output_name))
				if err != nil {
					return err
				}

				_, err = utils.Copy(ctx, f, reader)
				return err
			}

			err = copier(csv_tmpfile.Name(), "All "+
				path.Join(artifact, source)+".csv")
			if err != nil {
				report_err(err)
				continue
			}

			err = copier(json_tmpfile.Name(), "All "+
				path.Join(artifact, source)+".json")
			if err != nil {
				report_err(err)
			}
		}

		scope := artifacts.ScopeBuilder{
			Config:     config_obj,
			ACLManager: vql_subsystem.NewServerACLManager(config_obj, principal),
			Logger:     logging.NewPlainLogger(config_obj, &logging.ToolComponent),
			Env:        ordereddict.NewDict().Set("HuntId", hunt_id),
		}.Build()
		defer scope.Close()

		vql, _ := vfilter.Parse(
			"SELECT Flow.SessionId AS FlowId, ClientId " +
				"FROM hunt_flows(hunt_id=HuntId)")
		for row := range vql.Eval(ctx, scope) {
			flow_id := vql_subsystem.GetStringFromRow(scope, row, "FlowId")
			client_id := vql_subsystem.GetStringFromRow(scope, row, "ClientId")
			if flow_id == "" || client_id == "" {
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

type vfsFileDownloadRequest struct {
	ClientId string `schema:"client_id"`
	VfsPath  string `schema:"vfs_path,required"`
	Offset   int64  `schema:"offset"`
	Length   int    `schema:"length"`
	Encoding string `schema:"encoding"`
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

		file, err := file_store.GetFileStore(config_obj).ReadFile(request.VfsPath)
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
		filename := strings.Replace(request.VfsPath, "\"", "_", -1)
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
		filename := strings.Replace(request.VfsPath, "\"", "", -1)
		w.Header().Set("Content-Disposition", "attachment; filename="+
			url.PathEscape(filename+".zip"))
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

		client_path_manager := paths.NewClientPathManager(request.ClientId)

		db, _ := datastore.GetDB(config_obj)
		db.Walk(config_obj, client_path_manager.VFSDownloadInfoPath(request.VfsPath),
			func(path_name string) error {
				download_info := &flows_proto.VFSDownloadInfo{}
				err := db.GetSubject(config_obj, path_name, download_info)
				if err != nil {
					logger.Warn("Cant open %s: %v", path_name, err)
					return nil
				}

				fd, err := file_store_factory.ReadFile(download_info.VfsPath)
				if err != nil {
					return err
				}

				zh, err := zip_writer.Create(cleanPathForZip(path_name))
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

	file, err := file_store.GetFileStore(config_obj).ReadFile(vfs_path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	file.Seek(int64(offset), 0)

	result := &api_proto.VFSFileBuffer{
		Data: make([]byte, length),
	}

	n, err := file.Read(result.Data)
	if err != nil && errors.Cause(err) != io.EOF &&
		errors.Cause(err) != io.ErrUnexpectedEOF {
		return nil, err
	}

	result.Data = result.Data[:n]

	return result, nil
}
