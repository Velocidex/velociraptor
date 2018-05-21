package vql

import (
	"fmt"
	"reflect"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/vfilter"
)

// Returned as the result of the query.
type UploadResponse struct {
	Path   string
	FlowId string
	Size   uint64
}

// The upload plugin is a passthrough plugin which uploads the files
// to the server.

// Args:
//   - hits: A series of rows to upload. These are typically
//      subselects. The rows will be passed directly to the output of
//      the plugin.

// Example:
//   SELECT * from upload(hits= { SELECT FullPath FROM glob(globs=['/tmp/*.txt']) })
func MakeUploaderPlugin() *vfilter.GenericListPlugin {
	plugin := &vfilter.GenericListPlugin{
		PluginName: "upload",
		RowType:    UploadResponse{},
	}

	plugin.Function = func(
		scope *vfilter.Scope,
		args *vfilter.Dict) []vfilter.Row {
		result := []vfilter.Row{}

		// Extract the glob from the args.
		files, ok := args.Get("files")
		if ok {
			hits_slice := reflect.ValueOf(files)
			if hits_slice.Type().Kind() == reflect.Slice {
				for i := 0; i < hits_slice.Len(); i++ {
					value := hits_slice.Index(i).Interface()
					if path, ok := value.(string); ok {
						result = append(
							result, uploadFile(scope, path))
					}
				}
			}
		}
		vfilter.Debug(files)
		vfilter.Debug(result)
		return result
	}

	return plugin
}

func uploadFile(scope *vfilter.Scope, path string) *UploadResponse {
	responder_obj, ok := scope.Resolve("responder")
	if ok {
		responder := responder_obj.(*responder.Responder)
		result := &UploadResponse{
			Path:   path,
			FlowId: responder.SessionId(),
		}

		fmt.Println("Uploading ", path)

		return result
	}

	return nil

}
