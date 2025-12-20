/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2025 Rapid7 Inc.

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
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	errors "github.com/go-errors/errors"
	"github.com/gorilla/schema"

	"www.velocidex.com/golang/velociraptor/acls"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/api/authenticators"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/api/tables"
	api_utils "www.velocidex.com/golang/velociraptor/api/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
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
	FSComponents []string `schema:"fs_components"`
	Offset       int64    `schema:"offset"`
	Length       int      `schema:"length"`
	OrgId        string   `schema:"org_id"`

	// The caller can specify we detect the mime type. Only a few
	// types are supported.
	DetectMime bool `schema:"detect_mime"`

	// If set we pad the file out.
	Padding bool `schema:"padding"`

	// If set we filter binary chars to reveal only text
	TextFilter bool `schema:"text_filter"`
	Lines      int  `schema:"lines"`

	// Encapsulate the file in a zip file.
	ZipFile bool `schema:"zip"`
}

// URL format: /api/v1/DownloadVFSFile

// This URL allows the caller to download **any** member of the
// filestore (providing they have at least read permissions).
func vfsFileDownloadHandler() http.Handler {
	return api_utils.HandlerFunc(nil,
		func(w http.ResponseWriter, r *http.Request) {
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

			org_id = utils.NormalizedOrgId(org_id)

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

			users := services.GetUserManager()
			user_record, err := users.GetUserFromHTTPContext(r.Context())
			if err != nil {
				returnError(w, 403, err.Error())
				return
			}
			principal := user_record.Name
			permissions := acls.READ_RESULTS
			perm, err := services.CheckAccess(org_config_obj, principal, permissions)
			if !perm || err != nil {
				returnError(w, 403, "PermissionDenied")
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

				filename = utils.Base(request.VfsPath)

				// Uploads table has direct vfs paths
			} else if request.VfsPath != "" {
				path_spec, err = client_path_manager.GetUploadsFileFromVFSPath(
					request.VfsPath)
				if err != nil {
					returnError(w, 404, err.Error())
					return
				}
				filename = path_spec.Base()

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

			// We need to figure out the total size of the upload to set
			// in the Content Length header. There are three
			// possibilities:
			// 1. The file is not sparse
			// 2. The file is sparse and we are not padding.
			// 3. The file is sparse and we are padding it.
			var reader_at io.ReaderAt = utils.MakeReaderAtter(file)
			var total_size int

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

				total_size = calculateTotalSizeWithPadding(index)
			} else {
				total_size = calculateTotalReaderSize(file)
			}

			if request.TextFilter {
				output, next_offset, err := filterData(reader_at, request)
				if err != nil {
					returnError(w, 500, err.Error())
					return
				}

				w.Header().Set("Content-Disposition", "attachment; "+
					sanitizeFilenameForAttachment(filename))
				w.Header().Set("Content-Type",
					utils.GetMimeString(output, utils.AutoDetectMime(request.DetectMime)))
				w.Header().Set("Content-Range",
					fmt.Sprintf("bytes %d-%d/%d", request.Offset, next_offset, total_size))
				w.WriteHeader(200)

				_, _ = w.Write(output)
				return
			}

			// If the user requested the whole file, and also has password
			// set we send them a zip file with the entire thing
			if request.ZipFile {
				err = streamZipFile(r.Context(), org_config_obj, w, file, filename)
				if err == nil {
					return
				}
			}

			emitContentLength(w, int(request.Offset), int(request.Length), total_size)

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
					w.Header().Set("Content-Disposition", "attachment; "+
						sanitizeFilenameForAttachment(filename))
					w.Header().Set("Content-Type",
						utils.GetMimeString(buf[:n],
							utils.AutoDetectMime(request.DetectMime)))
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

// Read data from offset and filter it until the requested number of
// lines is found. This produces text only output, aka "strings"
func filterData(reader_at io.ReaderAt,
	request vfsFileDownloadRequest) (
	output []byte, next_offset int64, err error) {

	lines := 0
	required_lines := request.Lines
	if required_lines == 0 {
		required_lines = 25
	}
	offset := request.Offset

	buf := pool.Get().([]byte)
	defer pool.Put(buf)

	// This is a safety mechanism in case the file is mostly 0
	total_read := 0

	for {
		if total_read > 10*1024*1024 {
			break
		}

		n, err := reader_at.ReadAt(buf, offset)
		if err != nil && err != io.EOF {
			return nil, 0, err
		}

		if n <= 0 {
			break
		}

		total_read += n

		// Read the buffer and filter it collecting only printable
		// chars.
		for i := 0; i < n; i++ {
			c := buf[i]
			switch c {
			case 0:
				continue

			case '\n':
				lines++
				if required_lines <= lines {
					return output, offset + int64(i), nil
				}
				fallthrough

			default:
				if c >= 0x20 && c < 0x7f ||
					c == 10 || c == 13 || c == 9 {
					output = append(output, c)
				} else {
					output = append(output, '.')
				}
			}
		}
		offset += int64(n)
	}

	return output, offset, nil
}

func getRows(
	ctx context.Context,
	config_obj *config_proto.Config,
	request *api_proto.GetTableRequest,
	principal string) (
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
			ctx, config_obj, path_manager)

		return rs_reader.Rows(ctx), rs_reader.Close, log_path, err

	} else if request.Type == "STACK" {
		log_path = path_specs.NewUnsafeFilestorePath(
			utils.FilterSlice(request.StackPath, "")...).
			SetType(api.PATH_TYPE_FILESTORE_JSON)

		options, err := tables.GetTableOptions(request)
		if err != nil {
			return nil, nil, nil, err
		}

		rs_reader, err := result_sets.NewResultSetReaderWithOptions(
			ctx, config_obj, file_store_factory, log_path, options)
		if err != nil {
			return nil, nil, nil, err
		}

		return rs_reader.Rows(ctx), rs_reader.Close, log_path, err

	} else {
		log_path, err := tables.GetPathSpec(
			ctx, config_obj, request, principal)
		if err != nil {
			return nil, nil, nil, err
		}

		options, err := tables.GetTableOptions(request)
		if err != nil {
			return nil, nil, nil, err
		}

		rs_reader, err := result_sets.NewResultSetReaderWithOptions(
			ctx, config_obj, file_store_factory, log_path, options)
		if err != nil {
			return nil, nil, nil, err
		}
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

			base := ordereddict.NewDict().
				Set("ClientId", client_id).
				Set("Hostname", services.GetHostname(ctx, config_obj, client_id)).
				Set("FlowId", flow_id).
				Set("StartedTime", time.Unix(utils.GetInt64(row, "Timestamp"), 0))

			launcher, err := services.GetLauncher(config_obj)
			if err != nil {
				return base.Set("State", fmt.Sprintf("Unknown: %v", err))
			}

			flow, err := launcher.Storage().LoadCollectionContext(
				ctx, config_obj, client_id, flow_id)
			if err != nil {
				return base.Set("State", fmt.Sprintf("Unknown: %v", err))
			}

			return base.
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
	return api_utils.HandlerFunc(nil,
		func(w http.ResponseWriter, r *http.Request) {
			components := utils.SplitComponents(r.URL.Path)

			// make sure the prefix is correct
			for i, p := range prefix {
				if len(components) <= i || p != components[i] {
					returnError(w, 404, "Not Found")
					return
				}
			}

			path_spec := path_specs.FromGenericComponentList(components)

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

			// The following is not strictly necessary because this
			// function is behind the authenticator middleware which means
			// that if we get here the user is already authenticated and
			// has at least read permissions on this org. But we check
			// again to make sure we are resilient against possible
			// regressions in the authenticator code.
			users := services.GetUserManager()
			user_record, err := users.GetUserFromHTTPContext(r.Context())
			if err != nil {
				returnError(w, 404, err.Error())
				return
			}

			principal := user_record.Name
			permissions := acls.READ_RESULTS
			perm, err := services.CheckAccess(org_config_obj, principal, permissions)
			if !perm || err != nil {
				returnError(w, 403, "User is not allowed to read files.")
				return
			}

			file_store_factory := file_store.GetFileStore(org_config_obj)
			fd, err := file_store_factory.ReadFile(path_spec)
			if err != nil {
				returnError(w, 404, err.Error())
				return
			}

			buf := pool.Get().([]byte)
			defer pool.Put(buf)

			// Read the first buffer for mime detection.
			n, err := fd.Read(buf)
			if err != nil {
				returnError(w, 404, err.Error())
				return
			}

			// From here on we already sent the headers and we can
			// not really report an error to the client.
			w.Header().Set("Content-Disposition", "attachment; "+
				sanitizePathspecForAttachment(path_spec))

			w.Header().Set("Content-Type",
				utils.GetMimeString(buf[:n], utils.AutoDetectMime(true)))
			w.WriteHeader(200)
			_, _ = w.Write(buf[:n])

			// Copy the rest directly.
			_, _ = utils.Copy(r.Context(), w, fd)
		})
}

// Allowed chars in non extended names
const allowedChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ!#$&+-.^_`|~@'=()[]{}0123456789 "

func sanitizePathspecForAttachment(path_spec api.FSPathSpec) string {
	// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Content-Disposition
	// >  The string following filename should always be put into quotes;
	base_filename := path_spec.Base() + api.GetExtensionForFilestore(path_spec)
	return sanitizeFilenameForAttachment(base_filename)
}

func sanitizeFilenameForAttachment(base_filename string) string {
	// If the base filename contains path separator we use the last one
	if strings.Contains(base_filename, "/") {
		parts := strings.Split(base_filename, "/")
		base_filename = parts[len(parts)-1]
	}

	base_filename_ascii := []byte{}
	for _, c := range base_filename {
		if strings.Contains(allowedChars, string(c)) {
			base_filename_ascii = append(base_filename_ascii, byte(c))
		} else {
			base_filename_ascii = append(base_filename_ascii, '_')
		}
	}

	// The `filename*` parameter has to be encoded accroding to
	// RFC5987 without leading and trailing quotes or this fails in
	// Firefox.
	return fmt.Sprintf("filename*=utf-8''%s; filename=\"%s\" ",
		url.PathEscape(base_filename), url.PathEscape(string(base_filename_ascii)))
}

// Download the table as specified by the v1/GetTable API.
func downloadTable() http.Handler {
	return api_utils.HandlerFunc(nil,
		func(w http.ResponseWriter, r *http.Request) {
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

			user_record := GetUserInfo(r.Context(), org_config_obj)
			principal := user_record.Name

			// This should never happen!
			if principal == "" {
				returnError(w, 403, "Unauthenticated access.")
				return
			}

			row_chan, closer, log_path, err := getRows(
				r.Context(), org_config_obj, request, principal)
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
				w.Header().Set("Content-Disposition", "attachment; "+
					sanitizeFilenameForAttachment(download_name))
				w.Header().Set("Content-Type", "binary/octet-stream")
				w.WriteHeader(200)

				err := services.LogAudit(r.Context(),
					org_config_obj, principal, "DownloadTable",
					ordereddict.NewDict().
						Set("request", request).
						Set("remote", r.RemoteAddr))
				if err != nil {
					logger := logging.GetLogger(
						org_config_obj, &logging.FrontendComponent)
					logger.Error("<red>DownloadTable</> %v %v",
						principal, request)
				}

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
				w.Header().Set("Content-Disposition", "attachment; "+
					sanitizeFilenameForAttachment(download_name))
				w.Header().Set("Content-Type", "binary/octet-stream")
				w.WriteHeader(200)

				err = services.LogAudit(r.Context(),
					org_config_obj, principal, "DownloadTable",
					ordereddict.NewDict().
						Set("request", request).
						Set("remote", r.RemoteAddr))
				if err != nil {
					logger := logging.GetLogger(org_config_obj, &logging.FrontendComponent)
					logger.Error("<red>DownloadTable</> %v %v", principal, request)
				}

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

func vfsGetBuffer(config_obj *config_proto.Config, client_id string,
	vfs_path api.FSPathSpec, offset uint64, length uint32, padding bool) (
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
	if err == nil && padding && len(index.Ranges) > 0 {
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

	data, err := utils.ReadAllWithLimit(fd, constants.MAX_MEMORY)
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

func calculateTotalSizeWithPadding(index *actions_proto.Index) int {
	size := 0
	for _, r := range index.Ranges {
		size += int(r.Length)
	}
	return size
}

func calculateTotalReaderSize(reader api.FileReader) int {
	stat, err := reader.Stat()
	if err == nil {
		return int(stat.Size())
	}
	return 0
}

func emitContentLength(w http.ResponseWriter, offset int, req_length int, size int) {
	// Size is not known or 0, do not send a Content Length
	if size == 0 || offset > size {
		return
	}

	// How much data is available to read in the file.
	available := size - offset

	// If the user asked for less data than is available, then we will
	// return less, otherwise we only return how much data is
	// available.
	if req_length > BUFSIZE {
		req_length = BUFSIZE
	}

	// req_length of 0 means download the entire file without byte ranges.
	if req_length > 0 && req_length < available {
		available = req_length
	}

	w.Header().Set("Content-Length", fmt.Sprintf("%v", available))
}

func streamZipFile(
	ctx context.Context,
	config_obj *config_proto.Config,
	w http.ResponseWriter,
	file io.Reader, filename string) error {
	buf := pool.Get().([]byte)
	defer pool.Put(buf)

	w.Header().Set("Content-Disposition", "attachment; "+
		sanitizeFilenameForAttachment(filename+".zip"))
	w.Header().Set("Content-Type", "application/zip")
	w.WriteHeader(200)

	users := services.GetUserManager()
	user_record, err := users.GetUserFromHTTPContext(ctx)
	if err != nil {
		return err
	}

	// Get the user's preferences to set the container password
	password := ""
	options, err := users.GetUserOptions(ctx, user_record.Name)
	if err == nil {
		password = options.DefaultPassword
	}

	container, err := reporting.NewContainerFromWriter(
		fmt.Sprintf("HTTPDownload-%v", filename),
		config_obj, utils.NopWriteCloser{Writer: w}, password, 5, nil)
	if err != nil {
		return err
	}
	defer container.Close()

	file_writer, err := container.Create(filename, utils.Now())
	if err != nil {
		return err
	}
	defer file_writer.Close()

	for {
		n, err := file.Read(buf)
		if n == 0 || err == io.EOF {
			break
		}

		if err != nil {
			return err
		}

		_, err = file_writer.Write(buf[:n])
		if err != nil {
			return err
		}
	}

	return nil
}
