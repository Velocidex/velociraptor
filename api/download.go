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
	"html"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/gorilla/schema"
	errors "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/flows"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

var (
	pool = sync.Pool{
		New: func() interface{} {
			return make([]byte, 32*1024)
		},
	}
)

func returnError(w http.ResponseWriter, code int, message string) {
	w.WriteHeader(code)
	_, _ = w.Write([]byte(html.EscapeString(message)))
}

type vfsFileDownloadRequest struct {
	ClientId   string   `schema:"client_id"`
	VfsPath    string   `schema:"vfs_path"`
	Components []string `schema:"components[]"`
	Offset     int64    `schema:"offset"`
	Length     int      `schema:"length"`
	Encoding   string   `schema:"encoding"`
}

// URL format: /api/v1/DownloadVFSFile

// This URL allows the caller to download **any** member of the
// filestore (providing they have at least read permissions).
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

		var path_spec api.FSPathSpec
		client_path_manager := paths.NewClientPathManager(request.ClientId)

		// Uploads table has direct vfs paths
		if request.VfsPath != "" {
			path_spec, err = client_path_manager.GetUploadsFileFromVFSPath(
				request.VfsPath)
			if err != nil {
				returnError(w, 404, err.Error())
				return
			}

		} else {
			db, err := datastore.GetDB(config_obj)
			if err != nil {
				returnError(w, 404, err.Error())
				return
			}

			info_path_spec := client_path_manager.VFSDownloadInfoPath(
				request.Components)
			download_info := &flows_proto.VFSDownloadInfo{}

			err = db.GetSubject(config_obj, info_path_spec, download_info)
			if err != nil {
				returnError(w, 404, err.Error())
				return
			}
			path_spec = path_specs.NewUnsafeFilestorePath(
				download_info.Components...).
				SetType(api.PATH_TYPE_FILESTORE_ANY)

		}

		file, err := file_store.GetFileStore(config_obj).ReadFile(path_spec)
		if err != nil {
			returnError(w, 404, err.Error())
			return
		}
		defer file.Close()

		if r.Method == "HEAD" {
			returnError(w, 200, "Ok")
			return
		}

		var reader_at io.ReaderAt = &utils.ReaderAtter{Reader: file}

		index, err := getIndex(config_obj, path_spec)

		// If the file is sparse, we use the sparse reader.
		if err == nil && len(index.Ranges) > 0 {
			reader_at = &utils.RangedReader{
				ReaderAt: reader_at,
				Index:    index,
			}
		}

		offset := request.Offset

		// From here on we sent the headers and we can not
		// really report an error to the client.
		filename := strings.Replace(path_spec.Base(), "\"", "_", -1)
		w.Header().Set("Content-Disposition", "attachment; filename="+
			url.PathEscape(filename))
		w.Header().Set("Content-Type", "binary/octet-stream")
		w.WriteHeader(200)

		length_sent := 0
		buf := pool.Get().([]byte)
		defer pool.Put(buf)

		for {
			n, _ := reader_at.ReadAt(buf, offset)
			if n > 0 {
				if request.Length != 0 {
					length_to_send := request.Length - length_sent
					if n > length_to_send {
						n = length_to_send
					}
				}
				if n == 0 {
					return
				}

				_, err := w.Write(buf[:n])
				if err != nil {
					return
				}
				length_sent += n
				offset += int64(n)
			} else {
				return
			}
		}
	})
}

func getRows(
	ctx context.Context,
	config_obj *config_proto.Config,
	request *api_proto.GetTableRequest) (
	rows <-chan *ordereddict.Dict, close func(),
	log_path api.FSPathSpec, err error) {
	file_store_factory := file_store.GetFileStore(config_obj)

	// We want an event table.
	if request.Type == "CLIENT_EVENT" || request.Type == "SERVER_EVENT" {
		path_manager, err := artifacts.NewArtifactPathManager(
			config_obj, request.ClientId, request.FlowId,
			request.Artifact)
		if err != nil {
			return nil, nil, nil, err
		}

		log_path, err := path_manager.GetPathForWriting()
		if err != nil {
			return nil, nil, nil, err
		}

		rs_reader, err := result_sets.NewTimedResultSetReader(
			ctx, file_store_factory, path_manager)

		return rs_reader.Rows(ctx), rs_reader.Close, log_path, err

	} else {
		log_path, err := getPathSpec(config_obj, request)
		if err != nil {
			return nil, nil, nil, err
		}

		rs_reader, err := result_sets.NewResultSetReader(
			file_store_factory, log_path)

		return rs_reader.Rows(ctx), rs_reader.Close, log_path, err
	}
}

// The GUI transforms many of the raw tables we use - so when
// exporting we need to replicate this transformation, otherwise the
// results can be surprising.
func getTransformer(
	config_obj *config_proto.Config,
	in *api_proto.GetTableRequest) func(row *ordereddict.Dict) *ordereddict.Dict {
	if in.HuntId != "" && in.Type == "clients" {
		return func(row *ordereddict.Dict) *ordereddict.Dict {
			client_id := utils.GetString(row, "ClientId")
			flow_id := utils.GetString(row, "FlowId")

			flow, err := flows.LoadCollectionContext(config_obj, client_id, flow_id)
			if err != nil {
				flow = flows.NewCollectionContext(config_obj)
			}

			return ordereddict.NewDict().
				Set("ClientId", client_id).
				Set("Hostname", services.GetHostname(client_id)).
				Set("FlowId", flow_id).
				Set("StartedTime", time.Unix(utils.GetInt64(row, "Timestamp"), 0)).
				Set("State", flow.State.String()).
				Set("Duration", flow.ExecutionDuration/1000000000).
				Set("TotalBytes", flow.TotalUploadedBytes).
				Set("TotalRows", flow.TotalCollectedRows)
		}
	}

	// A unit transform.
	return func(row *ordereddict.Dict) *ordereddict.Dict { return row }
}

// Download the table as specified by the v1/GetTable API.
func downloadTable(config_obj *config_proto.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := &api_proto.GetTableRequest{}
		decoder := schema.NewDecoder()
		decoder.SetAliasTag("json")
		err := decoder.Decode(request, r.URL.Query())
		if err != nil {
			returnError(w, 404, err.Error())
			return
		}

		row_chan, closer, log_path, err := getRows(
			r.Context(), config_obj, request)
		if err != nil {
			returnError(w, 400, "Invalid request")
			return
		}
		defer closer()

		transform := getTransformer(config_obj, request)

		download_name := request.DownloadFilename
		if download_name == "" {
			download_name = strings.Replace(log_path.Base(), "\"", "", -1)
		}

		// Log an audit event.
		userinfo := GetUserInfo(r.Context(), config_obj)

		// This should never happen!
		if userinfo.Name == "" {
			returnError(w, 500, "Unauthenticated access.")
			return
		}

		switch request.DownloadFormat {
		case "csv":
			download_name = strings.TrimSuffix(download_name, ".json")
			download_name += ".csv"

			// From here on we already sent the headers and we can
			// not really report an error to the client.
			w.Header().Set("Content-Disposition", "attachment; filename="+
				url.PathEscape(download_name))
			w.Header().Set("Content-Type", "binary/octet-stream")
			w.WriteHeader(200)

			logger := logging.GetLogger(config_obj, &logging.Audit)
			logger.WithFields(logrus.Fields{
				"user":    userinfo.Name,
				"request": request,
				"remote":  r.RemoteAddr,
			}).Info("DownloadTable")

			scope := vql_subsystem.MakeScope()
			csv_writer := csv.GetCSVAppender(
				config_obj, scope, w, true /* write_headers */)
			for row := range row_chan {
				csv_writer.Write(
					filterColumns(request.Columns, transform(row)))
			}
			csv_writer.Close()

			// Output in jsonl by default.
		default:
			if !strings.HasSuffix(download_name, ".json") {
				download_name += ".json"
			}

			// From here on we already sent the headers and we can
			// not really report an error to the client.
			w.Header().Set("Content-Disposition", "attachment; filename="+
				url.PathEscape(download_name))
			w.Header().Set("Content-Type", "binary/octet-stream")
			w.WriteHeader(200)

			logger := logging.GetLogger(config_obj, &logging.Audit)
			logger.WithFields(logrus.Fields{
				"user":    userinfo.Name,
				"request": request,
				"remote":  r.RemoteAddr,
			}).Info("DownloadTable")

			for row := range row_chan {
				serialized, err := json.Marshal(
					filterColumns(request.Columns, transform(row)))
				if err != nil {
					return
				}

				// Write line delimited JSON
				_, _ = w.Write(serialized)
				_, _ = w.Write([]byte{'\n'})
			}
		}
	})
}

func vfsGetBuffer(
	config_obj *config_proto.Config,
	client_id string, vfs_path api.FSPathSpec, offset uint64, length uint32) (
	*api_proto.VFSFileBuffer, error) {

	file, err := file_store.GetFileStore(config_obj).ReadFile(vfs_path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var reader_at io.ReaderAt = &utils.ReaderAtter{Reader: file}

	result := &api_proto.VFSFileBuffer{
		Data: make([]byte, length),
	}

	// Try to get the index if it is there.
	index, err := getIndex(config_obj, vfs_path)

	// If the file is sparse, we use the sparse reader.
	if err == nil && len(index.Ranges) > 0 {
		reader_at = &utils.RangedReader{
			ReaderAt: reader_at,
			Index:    index,
		}
	}

	n, err := reader_at.ReadAt(result.Data, int64(offset))
	if err != nil && errors.Is(err, os.ErrNotExist) &&
		errors.Cause(err) != io.ErrUnexpectedEOF {
		return nil, err
	}

	result.Data = result.Data[:n]

	return result, nil
}

func getIndex(config_obj *config_proto.Config,
	vfs_path api.FSPathSpec) (*actions_proto.Index, error) {
	index := &actions_proto.Index{}

	file_store_factory := file_store.GetFileStore(config_obj)
	fd, err := file_store_factory.ReadFile(
		vfs_path.SetType(api.PATH_TYPE_FILESTORE_SPARSE_IDX))
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	data, err := ioutil.ReadAll(fd)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, &index)
	if err != nil {
		return nil, err
	}

	return index, nil
}

func filterColumns(columns []string, row *ordereddict.Dict) *ordereddict.Dict {
	if len(columns) == 0 {
		return row
	}

	new_row := ordereddict.NewDict()
	for _, column := range columns {
		value, _ := row.Get(column)
		new_row.Set(column, value)
	}
	return new_row
}
