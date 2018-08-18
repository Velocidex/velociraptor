package networking

import (
	"context"
	"www.velocidex.com/golang/velociraptor/glob"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

// We also offer a VQL function to manage the upload.
// Example: select upload(file=FullPath) from glob(globs="/bin/*")

// NOTE: Due to the order in which VQL is evaluated, VQL column
// transformations happen _BEFORE_ The where condition is
// applied. This means that an expression like:

// select upload(file=FullPath) from glob(globs="/bin/*") where Size > 100

// Will cause all files to be uploaded, even if their size is smaller
// than 100. You need to instead issue the following query to apply
// the WHERE clause filtering first, then upload the result:

// let files = select * from glob(globs="/bin/*") where Size > 100
// select upload(files=FullPath) from files
type UploadFunctionArgs struct {
	File string `vfilter:"required,field=file"`
}
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

		arg := &UploadFunctionArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("upload: %s", err.Error())
			return vfilter.Null{}
		}
		accessor := glob.OSFileSystemAccessor{}
		file, err := accessor.Open(arg.File)
		if err != nil {
			scope.Log("upload: Unable to open %s: %s",
				arg.File, err.Error())
			return &UploadResponse{
				Error: err.Error(),
			}
		}
		defer file.Close()

		stat, err := file.Stat()
		if err == nil && !stat.IsDir() {
			upload_response, err := uploader.Upload(
				scope, arg.File, file)
			if err != nil {
				return &UploadResponse{
					Error: err.Error(),
				}
			}
			return upload_response
		}
	}
	return vfilter.Null{}

}

func (self UploadFunction) Info(type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name: "upload",
		Doc: "Upload a file to the upload service. For a Velociraptor " +
			"client this will upload the file into the flow and store " +
			"it in the server's file store.",
		ArgType: type_map.AddType(&UploadFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&UploadFunction{})
}
