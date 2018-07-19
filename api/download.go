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
	config "www.velocidex.com/golang/velociraptor/config"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
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
		flow_details, err := getFlowDetails(config_obj, client_id, flow_id)
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
			result, err := getFlowResults(
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

		err = zip_writer.Close()
		if err != nil {
			return
		}
	})
}
