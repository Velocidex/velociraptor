/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2022 Rapid7 Inc.

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
	"bytes"
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
	errors "github.com/go-errors/errors"
	"github.com/gorilla/schema"

	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
	"www.velocidex.com/golang/velociraptor/acls"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/api/authenticators"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/api/tables"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/flows"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

const BUFSIZE = 1 * 1024 * 1024

var (
	pool = sync.Pool{
		New: func() interface{} {
			return make([]byte, BUFSIZE)
		},
	}
)

func returnError(w http.ResponseWriter, code int, message string) {
	w.WriteHeader(code)
	_, _ = w.Write([]byte(html.EscapeString(message)))
}

type vfsFileDownloadRequest struct {
	ClientId string `schema:"client_id"`

	// This is the path within the client VFS in the usual client path
	// notation - this is what is seen in the uploads table. We use
	// this field to determine the download attachment name.
	VfsPath string `schema:"vfs_path"`

	// This is the file store path to fetch.
	FSComponents []string `schema:"fs_components[]"`
	Offset       int64    `schema:"offset"`
	Length       int      `schema:"length"`
	OrgId        string   `schema:"org_id"`

	// The caller can specify we detect the mime type. Only a few
	// types are supported.
	DetectMime bool `schema:"detect_mime"`

	// If set we pad the file out.
	Padding bool `schema:"padding"`
}

// URL format: /api/v1/DownloadVFSFile

// This URL allows the caller to download **any** member of the
// filestore (providing they have at least read permissions).
func vfsFileDownloadHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := vfsFileDownloadRequest{}
		decoder := schema.NewDecoder()
		decoder.IgnoreUnknownKeys(true)

		err := decoder.Decode(&request, r.URL.Query())
		if err != nil {
			returnError(w, 403, "Error "+err.Error())
			return
		}

		org_id := request.OrgId
		if org_id == "" {
			org_id = authenticators.GetOrgIdFromRequest(r)
		}
		org_manager, err := services.GetOrgManager()
		if err != nil {
			returnError(w, 404, err.Error())
			return
		}

		org_config_obj, err := org_manager.GetOrgConfig(org_id)
		if err != nil {
			returnError(w, 404, err.Error())
			return
		}

		// Where to read from the file store
		var path_spec api.FSPathSpec

		// The filename for the attachment header.
		var filename string

		client_path_manager := paths.NewClientPathManager(request.ClientId)

		// Newer API calls pass the filestore components directly
		if len(request.FSComponents) > 0 {
			path_spec = path_specs.NewUnsafeFilestorePath(request.FSComponents...).
				SetType(api.PATH_TYPE_FILESTORE_ANY)

			base := utils.Base(request.VfsPath)
			filename = strings.Replace(base, "\"", "_", -1)

			// Uploads table has direct vfs paths
		} else if request.VfsPath != "" {
			path_spec, err = client_path_manager.GetUploadsFileFromVFSPath(
				request.VfsPath)
			if err != nil {
				returnError(w, 404, err.Error())
				return
			}
			filename = strings.Replace(path_spec.Base(), "\"", "_", -1)

		} else {
			// Just reject the request
			returnError(w, 404, "")
			return
		}

		file, err := file_store.GetFileStore(org_config_obj).ReadFile(path_spec)
		if err != nil {
			returnError(w, 404, err.Error())
			return
		}
		defer file.Close()

		if r.Method == "HEAD" {
			returnError(w, 200, "Ok")
			return
		}

		var reader_at io.ReaderAt = utils.MakeReaderAtter(file)

		index, err := getIndex(org_config_obj, path_spec)

		// If the file is sparse, we use the sparse reader.
		if err == nil && request.Padding && len(index.Ranges) > 0 {
			if !uploads.ShouldPadFile(org_config_obj, index) {
				returnError(w, 400, "Sparse file is too sparse - unable to pad")
				return
			}

			reader_at = &utils.RangedReader{
				ReaderAt: reader_at,
				Index:    index,
			}
		}

		offset := request.Offset

		// Read the first buffer now so we can report errors
		length_sent := 0
		headers_sent := false

		// Only allow limited size buffers to be requested by the user.
		var buf []byte
		if request.Length == 0 || request.Length >= BUFSIZE {
			buf = pool.Get().([]byte)
			defer pool.Put(buf)

		} else {
			buf = make([]byte, request.Length)
		}

		for {
			n, err := reader_at.ReadAt(buf, offset)
			if err != nil && err != io.EOF {
				// Only send errors if the headers have not yet been
				// sent.
				if !headers_sent {
					returnError(w, 500, err.Error())
					headers_sent = true
				}
				return
			}
			if request.Length != 0 {
				length_to_send := request.Length - length_sent
				if n > length_to_send {
					n = length_to_send
				}
			}
			if n <= 0 {
				return
			}

			// Write an ok status which includes the attachment name
			// but only if no other data was sent.
			if !headers_sent {
				w.Header().Set("Content-Disposition", "attachment; filename="+
					url.PathEscape(filename))
				w.Header().Set("Content-Type",
					detectMime(buf[:n], request.DetectMime))
				w.WriteHeader(200)
				headers_sent = true
			}

			written, err := w.Write(buf[:n])
			if err != nil {
				return
			}

			length_sent += written
			offset += int64(n)
		}
	})
}

func detectMime(buffer []byte, detect_mime bool) string {
	if detect_mime && len(buffer) > 8 {
		if 0 == bytes.Compare(
			[]byte("\x89\x50\x4E\x47\x0D\x0A\x1A\x0A"), buffer[:8]) {
			return "image/png"
		}
	}
	return "binary/octet-stream"
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
		path_manager, err := artifacts.NewArtifactPathManager(ctx,
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
		log_path, err := tables.GetPathSpec(ctx, config_obj, request)
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
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.GetTableRequest) func(row *ordereddict.Dict) *ordereddict.Dict {
	if in.HuntId != "" && in.Type == "clients" {
		return func(row *ordereddict.Dict) *ordereddict.Dict {
			client_id := utils.GetString(row, "ClientId")
			flow_id := utils.GetString(row, "FlowId")

			flow, err := flows.LoadCollectionContext(
				ctx, config_obj, client_id, flow_id)
			if err != nil {
				flow = flows.NewCollectionContext(ctx, config_obj)
			}

			return ordereddict.NewDict().
				Set("ClientId", client_id).
				Set("Hostname", services.GetHostname(ctx, config_obj, client_id)).
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

func downloadFileStore(prefix []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path_spec := paths.FSPathSpecFromClientPath(r.URL.Path)
		components := path_spec.Components()

		// make sure the prefix is correct
		for i, p := range prefix {
			if len(components) <= i || p != components[i] {
				returnError(w, 404, "Not Found")
				return
			}
		}

		org_id := authenticators.GetOrgIdFromRequest(r)
		org_manager, err := services.GetOrgManager()
		if err != nil {
			returnError(w, 404, err.Error())
			return
		}

		org_config_obj, err := org_manager.GetOrgConfig(org_id)
		if err != nil {
			returnError(w, 404, err.Error())
			return
		}

		file_store_factory := file_store.GetFileStore(org_config_obj)
		fd, err := file_store_factory.ReadFile(path_spec)
		if err != nil {
			returnError(w, 404, err.Error())
			return
		}

		// From here on we already sent the headers and we can
		// not really report an error to the client.
		w.Header().Set("Content-Disposition", "attachment; filename="+
			url.PathEscape(path_spec.Base())+api.GetExtensionForFilestore(path_spec))

		w.Header().Set("Content-Type", "binary/octet-stream")
		w.WriteHeader(200)

		utils.Copy(r.Context(), w, fd)
	})
}

// Download the table as specified by the v1/GetTable API.
func downloadTable() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := &api_proto.GetTableRequest{}
		decoder := schema.NewDecoder()
		decoder.IgnoreUnknownKeys(true)

		decoder.SetAliasTag("json")
		err := decoder.Decode(request, r.URL.Query())
		if err != nil {
			returnError(w, 404, err.Error())
			return
		}

		org_manager, err := services.GetOrgManager()
		if err != nil {
			returnError(w, 404, err.Error())
			return
		}

		org_config_obj, err := org_manager.GetOrgConfig(request.OrgId)
		if err != nil {
			returnError(w, 404, err.Error())
			return
		}

		row_chan, closer, log_path, err := getRows(
			r.Context(), org_config_obj, request)
		if err != nil {
			returnError(w, 400, "Invalid request")
			return
		}
		defer closer()

		transform := getTransformer(r.Context(), org_config_obj, request)

		download_name := request.DownloadFilename
		if download_name == "" {
			download_name = strings.Replace(log_path.Base(), "\"", "", -1)
		}

		// Log an audit event.
		user_record := GetUserInfo(r.Context(), org_config_obj)
		principal := user_record.Name

		// This should never happen!
		if principal == "" {
			returnError(w, 403, "Unauthenticated access.")
			return
		}

		permissions := acls.READ_RESULTS
		perm, err := services.CheckAccess(org_config_obj, principal, permissions)
		if !perm || err != nil {
			returnError(w, 403, "Unauthenticated access.")
			return
		}

		opts := json.GetJsonOptsForTimezone(request.Timezone)
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

			logging.LogAudit(org_config_obj, principal, "DownloadTable",
				logrus.Fields{
					"request": request,
					"remote":  r.RemoteAddr,
				})

			scope := vql_subsystem.MakeScope()
			csv_writer := csv.GetCSVAppender(
				org_config_obj, scope, w,
				csv.WriteHeaders, opts)
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

			logging.LogAudit(org_config_obj, principal, "DownloadTable",
				logrus.Fields{
					"request": request,
					"remote":  r.RemoteAddr,
				})

			for row := range row_chan {
				serialized, err := json.MarshalWithOptions(
					filterColumns(request.Columns, transform(row)),
					json.GetJsonOptsForTimezone(request.Timezone))
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

	var reader_at io.ReaderAt = utils.MakeReaderAtter(file)

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
	if err != nil &&
		errors.Is(err, os.ErrNotExist) &&
		!errors.Is(err, io.ErrUnexpectedEOF) {
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
