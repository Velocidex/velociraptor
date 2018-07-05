package vql

import (
	"os"
	"www.velocidex.com/golang/vfilter"
)

// The upload plugin uploads the files to the server using the
// configured uploader.

// Args:
//   - files: A series of filenames to upload.

// Example:
//   SELECT * from upload(files= { SELECT FullPath FROM glob(globs=['/tmp/*.txt']) })
func MakeUploaderPlugin() *vfilter.GenericListPlugin {
	plugin := &vfilter.GenericListPlugin{
		PluginName: "upload",
		RowType:    UploadResponse{},
	}

	plugin.Function = func(
		scope *vfilter.Scope,
		args *vfilter.Dict) []vfilter.Row {
		result := []vfilter.Row{}
		uploader_obj, ok := scope.Resolve("$uploader")
		if !ok {
			scope.Log("upload: Uploader not configured.")
			return result
		}

		uploader, ok := uploader_obj.(Uploader)
		if ok {
			// Extract the glob from the args.
			files, ok := vfilter.ExtractStringArray(scope, "files", args)
			if !ok {
				scope.Log("upload: Expecting a 'files' arg")
				return result
			}

			for _, filename := range files {
				file, err := os.Open(filename)
				if err != nil {
					scope.Log("upload: Unable to open %s: %s",
						filename, err.Error())
					continue
				}

				upload_response, err := uploader.Upload(
					scope, filename, file)
				if err != nil {
					continue
				}
				result = append(result, upload_response)
			}
		}
		return result
	}
	return plugin
}
