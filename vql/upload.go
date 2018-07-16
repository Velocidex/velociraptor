package vql

import (
	"context"
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

// We also offer a VQL function to manage the upload.
// Example: select upload(file=FullPath) from glob(globs="/bin/*")

// NOTE: Due to the order in which VQL is evaluated, VQL column
// transformations happen _BEFORE_ The where condition is
// applied. This means that an expression like:

// select upload(file=FullPath) from glob(globs="/bin/*") where Size > 100

// Will cause all files to be uploaded, even if their size is smaller
// than 100. You need to instead issue the following query to apply
// the where filtering first, then upload the result:

// let files = select * from glob(globs="/bin/*") where Size > 100
// select upload(files=FullPath) from files
type UploadFunction struct{}

func (self *UploadFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {

	uploader_obj, ok := scope.Resolve("$uploader")
	if !ok {
		scope.Log("upload: Uploader not configured.")
		return vfilter.Null{}
	}

	uploader, ok := uploader_obj.(Uploader)
	if ok {
		// Extract the glob from the args.
		filename, ok := vfilter.ExtractString("file", args)
		if !ok || filename == nil {
			scope.Log("upload: Expecting a 'files' arg")
			return vfilter.Null{}
		}

		file, err := os.Open(*filename)
		if err != nil {
			scope.Log("upload: Unable to open %s: %s",
				filename, err.Error())
			return vfilter.Null{}
		}

		upload_response, err := uploader.Upload(
			scope, *filename, file)
		if err != nil {
			return vfilter.Null{}
		}

		return upload_response
	}
	return vfilter.Null{}

}

func (self UploadFunction) Name() string {
	return "upload"
}
