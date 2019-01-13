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
	File     string `vfilter:"required,field=file"`
	Name     string `vfilter:"optional,field=name"`
	Accessor string `vfilter:"optional,field=accessor"`
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

		if arg.File == "" {
			return vfilter.Null{}
		}

		accessor := glob.GetAccessor(arg.Accessor, ctx)
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
		if err != nil {
			scope.Log("upload: Unable to stat %s: %v",
				arg.File, err)
		} else if !stat.IsDir() {
			upload_response, err := uploader.Upload(
				scope, arg.File, arg.Accessor, arg.Name, file)
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

func (self UploadFunction) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name: "upload",
		Doc: "Upload a file to the upload service. For a Velociraptor " +
			"client this will upload the file into the flow and store " +
			"it in the server's file store.",
		ArgType: type_map.AddType(scope, &UploadFunctionArgs{}),
	}
}

type UploadPluginArgs struct {
	Files    []string `vfilter:"required,field=files"`
	Accessor string   `vfilter:"optional,field=accessor"`
}

type UploadPlugin struct{}

func (self *UploadPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	arg := &UploadPluginArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("upload: %s", err.Error())
		close(output_chan)
		return output_chan
	}

	uploader_obj, _ := scope.Resolve("$uploader")
	uploader, ok := uploader_obj.(Uploader)
	if !ok {
		// If the uploader is not configured, we need to do
		// nothing.
		close(output_chan)
		return output_chan
	}

	go func() {
		defer close(output_chan)

		accessor := glob.GetAccessor(arg.Accessor, ctx)
		for _, filename := range arg.Files {
			file, err := accessor.Open(filename)
			if err != nil {
				scope.Log("upload: Unable to open %s: %s",
					filename, err.Error())
				continue
			}

			upload_response, err := uploader.Upload(
				scope, filename, arg.Accessor, filename, file)
			if err != nil {
				scope.Log("upload: Failed to upload %s: %s",
					filename, err.Error())
				continue
			}
			output_chan <- upload_response
		}
	}()
	return output_chan
}

func (self UploadPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "upload",
		Doc:     "Upload files to the server.",
		RowType: type_map.AddType(scope, &UploadResponse{}),
		ArgType: type_map.AddType(scope, &UploadPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&UploadFunction{})
	vql_subsystem.RegisterPlugin(&UploadPlugin{})
}
