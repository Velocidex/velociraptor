// Implement flow result downloads. For now we do not use gRPC for
// this but implement it directly in the API.
package api

import (
	"archive/zip"
	"fmt"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/ptypes"
	"io"
	"net/http"
	"strings"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config "www.velocidex.com/golang/velociraptor/config"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/flows"
	utils "www.velocidex.com/golang/velociraptor/testing"
)

func returnError(w http.ResponseWriter, code int, message string) {
	w.WriteHeader(code)
	w.Write([]byte(message))
}

// URL format: /api/v1/download/<client_id>/<flow_id>
func flowResultDownloadHandler(
	config_obj *config.Config) http.Handler {
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
	config_obj *config.Config) http.Handler {
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
			utils.Debug(offset)
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
			utils.Debug(offset)
			hunt_results, err := flows.GetHuntInfos(
				config_obj,
				&api_proto.GetHuntResultsRequest{
					HuntId: hunt_id[0],
					Offset: offset,
					Count:  page_size,
				})
			if err != nil {
				utils.Debug(err)
				break
			}
			if len(hunt_results.Items) == 0 {
				break
			}
			offset += uint64(len(hunt_results.Items))

			for _, hunt_info := range hunt_results.Items {
				utils.Debug(hunt_info)
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
