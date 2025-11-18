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
package networking

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/file"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

// We also offer a VQL function to manage the upload.
// Example: select upload(file=FullPath) from glob(globs="/bin/*")

type UploadFunctionArgs struct {
	File     *accessors.OSPath `vfilter:"required,field=file,doc=The file to upload"`
	Name     *accessors.OSPath `vfilter:"optional,field=name,doc=The name of the file that should be stored on the server"`
	Accessor string            `vfilter:"optional,field=accessor,doc=The accessor to use"`
	Mtime    vfilter.Any       `vfilter:"optional,field=mtime,doc=Modified time to record"`
	Atime    vfilter.Any       `vfilter:"optional,field=atime,doc=Access time to record"`
	Ctime    vfilter.Any       `vfilter:"optional,field=ctime,doc=Change time to record"`
	Btime    vfilter.Any       `vfilter:"optional,field=btime,doc=Birth time to record"`
}

type UploadFunction struct{}

func (self *UploadFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "upload", args)()

	uploader, ok := artifacts.GetUploader(scope)
	if !ok {
		scope.Log("upload: Uploader not configured.")
		return vfilter.Null{}
	}

	arg := &UploadFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("upload: %v", err)
		return vfilter.Null{}
	}

	if arg.File == nil {
		return vfilter.Null{}
	}

	accessor_name := arg.Accessor
	if accessor_name == "" {
		accessor_name = "auto"
	}

	accessor, err := accessors.GetAccessor(accessor_name, scope)
	if err != nil {
		scope.Log("upload: %v", err)
		return &uploads.UploadResponse{
			Error: err.Error(),
		}
	}

	file, err := accessor.OpenWithOSPath(arg.File)
	if err != nil {
		scope.Log("upload: Unable to open %s: %s",
			arg.File, err.Error())
		return &uploads.UploadResponse{
			Error: err.Error(),
		}
	}
	defer file.Close()

	stat, err := accessor.LstatWithOSPath(arg.File)
	if err != nil {
		scope.Log("upload: Unable to stat %s: %v",
			arg.File, err)
		return vfilter.Null{}
	}

	mtime, err := functions.TimeFromAny(ctx, scope, arg.Mtime)
	if err != nil {
		mtime = stat.ModTime()
	}

	atime, _ := functions.TimeFromAny(ctx, scope, arg.Atime)
	ctime, _ := functions.TimeFromAny(ctx, scope, arg.Ctime)
	btime, _ := functions.TimeFromAny(ctx, scope, arg.Btime)

	upload_response, err := uploader.Upload(
		ctx, scope, arg.File,
		accessor_name,
		arg.Name,
		stat.Size(), // Expected size.
		mtime, atime, ctime, btime, stat.Mode(),
		file)
	if err != nil {
		return &uploads.UploadResponse{
			Error: err.Error(),
		}
	}
	return upload_response.AsDict()
}

func (self UploadFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name: "upload",
		Doc: "Upload a file to the upload service. For a Velociraptor " +
			"client this will upload the file into the flow and store " +
			"it in the server's file store.",
		ArgType:  type_map.AddType(scope, &UploadFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
		Version:  3,
	}
}

type UploadDirectoryFunctionArgs struct {
	File       *accessors.OSPath `vfilter:"required,field=file,doc=The file to upload"`
	Name       *accessors.OSPath `vfilter:"optional,field=name,doc=Filename to be stored within the output directory"`
	Accessor   string            `vfilter:"optional,field=accessor,doc=The accessor to use"`
	OutputPath string            `vfilter:"required,field=output,doc=An output directory to store files in."`

	Mtime vfilter.Any `vfilter:"optional,field=mtime,doc=Modified time to set the output file."`
	Atime vfilter.Any `vfilter:"optional,field=atime,doc=Access time to set the output file."`
	Ctime vfilter.Any `vfilter:"optional,field=ctime,doc=Change time to set the output file."`
	Btime vfilter.Any `vfilter:"optional,field=btime,doc=Birth time to set the output file."`
}

type UploadDirectoryFunction struct{}

func (self *UploadDirectoryFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "upload_directory", args)()

	arg := &UploadDirectoryFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("upload_directory: %s", err.Error())
		return vfilter.Null{}
	}

	// Make sure we are allowed to write there.
	err = file.CheckPath(arg.OutputPath)
	if err != nil {
		scope.Log("upload_directory: %v", err)
		return vfilter.Null{}
	}

	uploader := &uploads.FileBasedUploader{
		UploadDir: arg.OutputPath,
	}

	if arg.File == nil {
		return vfilter.Null{}
	}

	// We are going to write on the filesystem.
	err = vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_WRITE)
	if err != nil {
		scope.Log("upload_directory: %s", err)
		return vfilter.Null{}
	}

	accessor, err := accessors.GetAccessor(arg.Accessor, scope)
	if err != nil {
		scope.Log("upload_directory: %v", err)
		return &uploads.UploadResponse{
			Error: err.Error(),
		}
	}

	file, err := accessor.OpenWithOSPath(arg.File)
	if err != nil {
		scope.Log("upload_directory: Unable to open %s: %s",
			arg.File.String(), err.Error())
		return &uploads.UploadResponse{
			Error: err.Error(),
		}
	}
	defer file.Close()

	stat, err := accessor.LstatWithOSPath(arg.File)
	if err != nil {
		scope.Log("upload_directory: Unable to stat %s: %v",
			arg.File.String(), err)
		return vfilter.Null{}
	}

	if stat.IsDir() {
		return vfilter.Null{}
	}

	// Stat only has a single time.
	mtime, err := functions.TimeFromAny(ctx, scope, arg.Mtime)
	if err != nil {
		mtime = stat.ModTime()
	}

	atime, _ := functions.TimeFromAny(ctx, scope, arg.Atime)
	ctime, _ := functions.TimeFromAny(ctx, scope, arg.Ctime)
	btime, _ := functions.TimeFromAny(ctx, scope, arg.Btime)

	upload_response, err := uploader.Upload(
		ctx, scope, arg.File,
		arg.Accessor,
		arg.Name,
		stat.Size(), // Expected size.
		mtime, atime, ctime, btime, stat.Mode(),
		file)
	if err != nil {
		return &uploads.UploadResponse{
			Error: err.Error(),
		}
	}
	return upload_response
}

func (self UploadDirectoryFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "upload_directory",
		Doc:      "Upload a file to an upload directory. The final filename will be the output directory path followed by the filename path.",
		ArgType:  type_map.AddType(scope, &UploadDirectoryFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_WRITE).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&UploadFunction{})
	vql_subsystem.RegisterFunction(&UploadDirectoryFunction{})
}
