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

// NOTE: Most Downloads are now split into two phases - the first is
// creation performed by the vql functions create_flow_download() and
// create_hunt_download(). The GUI can then fetch them directly
// through a file store handler installed on the "/downloads/" path.
package api

import (
	"archive/zip"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/schema"
	errors "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/utils"
)

func returnError(w http.ResponseWriter, code int, message string) {
	w.WriteHeader(code)
	w.Write([]byte(message))
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

		client_id := request.ClientId
		hostname := GetHostname(config_obj, client_id)
		client_path_manager := paths.NewClientPathManager(client_id)

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

				zh, err := zip_writer.Create(utils.CleanPathForZip(
					path_name, client_id, hostname))
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
