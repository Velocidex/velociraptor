//go:build extras

// References
// https://learn.microsoft.com/en-us/azure/storage/common/storage-sas-overview
// In order to create a suitable SAS URL:
// 1. Create a Storage Account
// 2. Under the IAM tab Add/Add role assignment, Add the Storage Blob Data Contributor role
// 3. Under "Containers" Create a new container
// 4. Right click on the container and select Generate SAS
// 5. Signing method is User Delegation, Permissions are: Read, Write, Create
// 6. Copy the "Blob SAS URL" into the Velociraptor UI

package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Velocidex/ordereddict"
	"github.com/hirochachacha/go-smb2"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/smb"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type SMBUploadArgs struct {
	File          *accessors.OSPath `vfilter:"required,field=file,doc=The file to upload"`
	Name          *accessors.OSPath `vfilter:"optional,field=name,doc=The name of the file that should be stored on the server"`
	Accessor      string            `vfilter:"optional,field=accessor,doc=The accessor to use"`
	Username      string            `vfilter:"optional,field=username,doc=The SMB username to login as (if not provided we use the SMB_CREDENTIALS env)"`
	Password      string            `vfilter:"optional,field=password,doc=The SMB password to login as (if not provided we use the SMB_CREDENTIALS env)"`
	ServerAddress string            `vfilter:"required,field=server_address,doc=The SMB server address and optionally port followed by the share name (e.g. \\\\192.168.1.1:445\\ShareName)"`
}

type SMBUploadFunction struct{}

func (self *SMBUploadFunction) getSession(
	scope vfilter.Scope, full_path *accessors.OSPath) (
	*smb2.Session, func(), error) {
	if len(full_path.Components) == 0 {
		return nil, nil, errors.New("First path component for smb accessor must be a server name or IP")
	}

	cache, pres := vql_subsystem.CacheGet(scope, smb.SMB_TAG).(*smb.SMBMountCache)
	if !pres {
		cache = smb.NewSMBMountCache(scope)
		vql_subsystem.CacheSet(scope, smb.SMB_TAG, cache)
	}

	server_name := full_path.Components[0]
	connection, closer, err := cache.GetHandle(server_name)
	if err != nil {
		return nil, nil, err
	}

	return connection.Session(), closer, nil
}

func (self *SMBUploadFunction) getMount(
	scope vfilter.Scope, full_path *accessors.OSPath) (
	*smb2.Share, string, func(), error) {
	if len(full_path.Components) < 2 {
		return nil, "", nil, errors.New("upload_smb requires at least a server name and share name.")
	}

	session, closer, err := self.getSession(scope, full_path)
	if err != nil {
		return nil, "", nil, err
	}

	share := full_path.Components[1]
	fs, err := session.Mount(share)
	if err != nil {
		return nil, "", nil, err
	}

	directory := "."
	if len(full_path.Components) > 2 {
		directory = strings.Join(full_path.Components[2:], "\\")
	}

	return fs, directory, closer, nil
}

func (self *SMBUploadFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "upload_smb", args)()

	arg := &SMBUploadArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("upload_smb: %v", err)
		return vfilter.Null{}
	}

	accessor, err := accessors.GetAccessor(arg.Accessor, scope)
	if err != nil {
		scope.Log("upload_smb: %v", err)
		return vfilter.Null{}
	}

	// Parse the server address as an SMB path
	root_path := &accessors.OSPath{Manipulator: &smb.SMBPathManipulator{}}
	server_path, err := root_path.Parse(arg.ServerAddress)
	if err != nil {
		scope.Log("upload_smb: %v", err)
		return vfilter.Null{}
	}

	if len(server_path.Components) < 2 {
		scope.Log("upload_smb: server_address should contain a hostname and a share name (e.g. //192.168.1.1/ShareName)")
		return vfilter.Null{}
	}

	file, err := accessor.OpenWithOSPath(arg.File)
	if err != nil {
		scope.Log("upload_smb: Unable to open %s: %v", arg.File, err)
		return &vfilter.Null{}
	}
	defer file.Close()

	if arg.Name == nil {
		arg.Name = arg.File
	}

	stat, err := accessor.LstatWithOSPath(arg.File)
	if err != nil {
		scope.Log("upload_smb: Unable to stat %s: %v",
			arg.File, err)
	} else if stat.IsDir() {
		return vfilter.Null{}
	}

	sub_scope := scope.Copy()

	// Pass credentials to the library in the scope if the credentials
	// are specified, otherwise we allow credentials to be passed from
	// the query scope.
	if arg.Username != "" && arg.Password != "" {
		// Credentials in SMB go with the hostname because they refer
		// to the local or domain user we use to log into the
		// share. Therefore all shares on the same hostname will have
		// the same credentials and so we can cache under the hostname
		// key.
		hostname := server_path.Components[0]
		sub_scope.AppendVars(ordereddict.NewDict().
			Set("SMB_CREDENTIALS", ordereddict.NewDict().
				Set(hostname, fmt.Sprintf("%s:%s", arg.Username, arg.Password))))
	}

	// Abort uploading when the scope is destroyed.
	sub_ctx, cancel := context.WithCancel(ctx)
	// Cancel the smb upload when the scope destroys.
	_ = scope.AddDestructor(cancel)
	upload_response, err := self.upload_smb(
		sub_ctx, sub_scope, file, arg.Name, arg.Username, arg.Password,
		server_path, uint64(stat.Size()))
	if err != nil {
		scope.Log("upload_smb: %v", err)
		return &uploads.UploadResponse{
			Error: err.Error(),
		}
	}
	return upload_response
}

func (self *SMBUploadFunction) upload_smb(ctx context.Context, scope vfilter.Scope,
	reader accessors.ReadSeekCloser,
	name *accessors.OSPath, username,
	password string, server_address *accessors.OSPath,
	size uint64) (*uploads.UploadResponse, error) {

	dest := server_address.Append(name.Components...)
	scope.Log("upload_smb: Uploading %v", dest)

	// Get the cached session
	fs, path, closer, err := self.getMount(scope, dest)
	if err != nil {
		return nil, err
	}
	defer closer()

	file_obj, err := fs.Create(path)
	if err != nil {
		return nil, err
	}
	defer file_obj.Close()

	n, err := utils.Copy(ctx, file_obj, reader)
	if err != nil {
		return &uploads.UploadResponse{
			Error: err.Error(),
		}, err
	}

	return &uploads.UploadResponse{
		Path: dest.String(),
		Size: uint64(n),
	}, nil
}

func (self SMBUploadFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "upload_smb",
		Doc:      "Upload files using the SMB file share protocol.",
		ArgType:  type_map.AddType(scope, &SMBUploadArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&SMBUploadFunction{})
}
