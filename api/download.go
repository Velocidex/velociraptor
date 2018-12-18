// Implement downloads. For now we do not use gRPC for this but
// implement it directly in the API.
package api

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/ptypes"
	"github.com/gorilla/schema"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/flows"
	"www.velocidex.com/golang/velociraptor/logging"
)

func returnError(w http.ResponseWriter, code int, message string) {
	w.WriteHeader(code)
	w.Write([]byte(message))
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

		// From here on we sent the headers and we can not
		// really report an error to the client.
		w.Header().Set("Content-Disposition", "attachment; filename='"+flow_id+".zip'")
		w.Header().Set("Content-Type", "binary/octet-stream")
		w.WriteHeader(200)

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

		// Serialize all flow results.
		offset := uint64(0)
		page_size := uint64(50)
		for {
			result, err := flows.GetFlowResults(
				config_obj, client_id, flow_id,
				offset, page_size)
			if err != nil {
				return
			}

			var idx int = 0
			var item *crypto_proto.GrrMessage
			for idx, item = range result.Items {
				var tmp ptypes.DynamicAny
				err := ptypes.UnmarshalAny(item.Payload, &tmp)
				if err != nil {
					continue
				}

				payload := tmp.Message
				vql_response, pres := payload.(*actions_proto.VQLResponse)
				if pres {
					path := fmt.Sprintf(
						"result/request-%03d/response-%03d/part-%03d",
						item.RequestId,
						item.ResponseId,
						vql_response.Part)
					result_json, err := marshaler.MarshalToString(vql_response)
					if err != nil {
						continue
					}

					f, err := zip_writer.Create(path)
					if err != nil {
						return
					}

					_, err = f.Write([]byte(result_json))
					if err != nil {
						return
					}
				}

			}

			if uint64(idx) < page_size {
				break
			}
			offset += uint64(idx)
		}

		file_store_factory := file_store.GetFileStore(config_obj)

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
	})
}

// URL format: /api/v1/DownloadHuntResults
func huntResultDownloadHandler(
	config_obj *api_proto.Config) http.Handler {
	logger := logging.NewLogger(config_obj)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hunt_id, pres := r.URL.Query()["hunt_id"]
		if !pres || len(hunt_id) == 0 {
			returnError(w, 404, "Hunt id should be specified.")
			return
		}
		hunt_details, err := flows.GetHunt(
			config_obj,
			&api_proto.GetHuntRequest{HuntId: hunt_id[0]})
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
		w.Header().Set("Content-Disposition", "attachment; filename='"+hunt_id[0]+".zip'")
		w.Header().Set("Content-Type", "binary/octet-stream")
		w.WriteHeader(200)

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

		// Serialize all flow results.
		offset := uint64(0)
		page_size := uint64(50)
		for {
			result, err := flows.GetHuntResults(
				config_obj,
				&api_proto.GetHuntResultsRequest{
					HuntId: hunt_id[0],
					Offset: offset,
					Count:  page_size,
				})
			if err != nil {
				return
			}
			if len(result.Items) == 0 {
				break
			}
			offset += uint64(len(result.Items))
			for _, item := range result.Items {
				var tmp ptypes.DynamicAny
				err := ptypes.UnmarshalAny(item.Payload, &tmp)
				if err != nil {
					continue
				}

				payload := tmp.Message
				vql_response, pres := payload.(*actions_proto.VQLResponse)
				if pres {
					path := fmt.Sprintf(
						"result/client-%s/request-%03d/response-%03d/part-%03d",
						item.Source,
						item.RequestId,
						item.ResponseId,
						vql_response.Part)
					result_json, err := marshaler.MarshalToString(vql_response)
					if err != nil {
						continue
					}

					f, err := zip_writer.Create(path)
					if err != nil {
						return
					}

					_, err = f.Write([]byte(result_json))
					if err != nil {
						return
					}
				}

			}
		}

		file_store_factory := file_store.GetFileStore(config_obj)
		offset = uint64(0)
		page_size = uint64(50)
		for {
			hunt_results, err := flows.GetHuntInfos(
				config_obj,
				&api_proto.GetHuntResultsRequest{
					HuntId: hunt_id[0],
					Offset: offset,
					Count:  page_size,
				})
			if err != nil {
				logger.Error("huntResultDownloadHandler: ", err)
				break
			}
			if len(hunt_results.Items) == 0 {
				break
			}
			offset += uint64(len(hunt_results.Items))

			for _, hunt_info := range hunt_results.Items {
				// Get all file uploads
				for _, upload_name := range hunt_info.Result.UploadedFiles {
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
			}
		}
	})
}

type vfsFileDownloadRequest struct {
	ClientId string `schema:"client_id,required"`
	VfsPath  string `schema:"vfs_path,required"`
	Offset   int64  `schema:"offset"`
	Length   int    `schema:"length"`
	Encoding string `schema:"encoding"`
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

		vfs_path := path.Clean(request.VfsPath)
		if strings.HasPrefix(vfs_path, "/monitoring/") ||
			strings.HasPrefix(vfs_path, "/artifacts/") {
			vfs_path = path.Join(
				"clients", request.ClientId, vfs_path)
		} else {
			vfs_path = path.Join(
				"clients", request.ClientId, "vfs_files", vfs_path)
		}

		file, err := file_store.GetFileStore(config_obj).ReadFile(vfs_path)
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
		filename := strings.Replace(path.Dir(vfs_path), "\"", "_", -1)
		w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
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
