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
	"net/url"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/networking"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type AzureUploadArgs struct {
	File     *accessors.OSPath `vfilter:"required,field=file,doc=The file to upload"`
	Name     string            `vfilter:"optional,field=name,doc=The name of the file that should be stored on the server"`
	Accessor string            `vfilter:"optional,field=accessor,doc=The accessor to use"`
	SASURL   string            `vfilter:"required,field=sas_url,doc=A SAS URL to use for upload to the container."`
}

type AzureUploadFunction struct{}

func (self *AzureUploadFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "upload_azure", args)()

	arg := &AzureUploadArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("upload_azure: %v", err)
		return vfilter.Null{}
	}

	accessor, err := accessors.GetAccessor(arg.Accessor, scope)
	if err != nil {
		scope.Log("upload_azure: %v", err)
		return vfilter.Null{}
	}

	file, err := accessor.OpenWithOSPath(arg.File)
	if err != nil {
		scope.Log("upload_azure: Unable to open %s: %v", arg.File, err)
		return &vfilter.Null{}
	}
	defer file.Close()

	if arg.Name == "" {
		arg.Name = arg.File.String()
	}

	stat, err := accessor.LstatWithOSPath(arg.File)
	if err != nil {
		scope.Log("upload_azure: Unable to stat %s: %v",
			arg.File, err)
	} else if !stat.IsDir() {
		// Abort uploading when the scope is destroyed.
		sub_ctx, cancel := context.WithCancel(ctx)
		// Cancel the s3 upload when the scope destroys.
		_ = scope.AddDestructor(cancel)
		upload_response, err := upload_azure(
			sub_ctx, scope, file,
			arg.Name, arg.SASURL,
			uint64(stat.Size()))
		if err != nil {
			scope.Log("upload_azure: %v", err)
			// Relay the error in the UploadResponse
			return upload_response
		}
		return upload_response
	}

	return vfilter.Null{}
}

func upload_azure(
	ctx context.Context, scope vfilter.Scope,
	reader accessors.ReadSeekCloser,
	name string, sas_url string, size uint64) (
	*uploads.UploadResponse, error) {

	scope.Log("upload_azure: Uploading %v", name)

	options := &azblob.ClientOptions{}

	config_obj, ok := artifacts.GetConfig(scope)
	if ok {
		url_obj, err := url.Parse(sas_url)
		if err != nil {
			return nil, err
		}

		transport, _, err := networking.GetHttpClient(ctx, config_obj,
			scope, &networking.HttpPluginRequest{
				Url: []string{sas_url},
			}, url_obj)
		if err != nil {
			return nil, err
		}
		options.Transport = transport
	}

	azClient, err := azblob.NewClientWithNoCredential(sas_url, options)
	if err != nil {
		return nil, err
	}

	_, err = azClient.UploadStream(ctx,
		"", // The container name is already encoded in the SAS URL
		name, reader, &azblob.UploadStreamOptions{})
	if err != nil {
		return nil, err
	}

	// All good! report the outcome.
	response := &uploads.UploadResponse{
		Path: name,
	}

	response.Size = size
	return response, nil
}

func (self AzureUploadFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "upload_azure",
		Doc:      "Upload files to Azure Blob Storage Service.",
		ArgType:  type_map.AddType(scope, &AzureUploadArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&AzureUploadFunction{})
}
