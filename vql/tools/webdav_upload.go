//go:build extras
// +build extras

package tools

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/uploads"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/networking"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type WebDAVUploadArgs struct {
	File              *accessors.OSPath `vfilter:"required,field=file,doc=The file to upload"`
	Name              string            `vfilter:"optional,field=name,doc=The name that the file should have on the server"`
	Accessor          string            `vfilter:"optional,field=accessor,doc=The accessor to use"`
	Url               string            `vfilter:"required,field=url,doc=The WebDAV url"`
	BasicAuthUser     string            `vfilter:"optional,field=basic_auth_user,doc=The username to use in HTTP basic auth"`
	BasicAuthPassword string            `vfilter:"optional,field=basic_auth_password,doc=The password to use in HTTP basic auth"`
	NoVerifyCert      bool              `vfilter:"optional,field=noverifycert,doc=Skip TLS Verification (deprecated in favor of SkipVerify)"`
	SkipVerify        bool              `vfilter:"optional,field=skip_verify,doc=Skip TLS Verification"`
}

type WebDAVUploadFunction struct{}

func (self *WebDAVUploadFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &WebDAVUploadArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("upload_webdav: %s", err.Error())
		return vfilter.Null{}
	}

	if arg.NoVerifyCert {
		scope.Log("upload_webdav: NoVerifyCert is deprecated, please use SkipVerify instead")
	}

	err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
	if err != nil {
		scope.Log("upload_webdav: %s", err)
		return vfilter.Null{}
	}

	accessor, err := accessors.GetAccessor(arg.Accessor, scope)
	if err != nil {
		scope.Log("upload_webdav: %v", err)
		return vfilter.Null{}
	}

	file, err := accessor.OpenWithOSPath(arg.File)
	if err != nil {
		scope.Log("upload_webdav: Unable to open %s: %s",
			arg.File, err.Error())
		return &vfilter.Null{}
	}
	defer file.Close()

	if arg.Name == "" {
		arg.Name = arg.File.String()
	}

	stat, err := accessor.LstatWithOSPath(arg.File)
	if err != nil {
		scope.Log("upload_webdav: Unable to stat %s: %v",
			arg.File, err)
	} else if !stat.IsDir() {
		// Abort uploading when the scope is destroyed.
		sub_ctx, cancel := context.WithCancel(ctx)
		_ = scope.AddDestructor(cancel)

		upload_response, err := upload_webdav(
			sub_ctx, scope, file, stat.Size(),
			arg.Name,
			arg.Url,
			arg.BasicAuthUser,
			arg.BasicAuthPassword,
			arg.NoVerifyCert || arg.SkipVerify)
		if err != nil {
			scope.Log("upload_webdav: %v", err)
			return vfilter.Null{}
		}
		return upload_response
	}

	return vfilter.Null{}
}

func upload_webdav(ctx context.Context, scope vfilter.Scope,
	reader io.Reader,
	contentLength int64,
	name string,
	webdavUrl string,
	basicAuthUser string,
	basicAuthPassword string,
	skipVerify bool) (
	*uploads.UploadResponse, error) {

	scope.Log("upload_webdav: Uploading %v to %v", name, webdavUrl)

	parsedUrl, err := url.Parse(webdavUrl)
	if err != nil {
		return &uploads.UploadResponse{
			Error: err.Error(),
		}, err
	}
	parsedUrl.Path = path.Join(parsedUrl.Path, name)

	config_obj, ok := artifacts.GetConfig(scope)
	if !ok {
		return nil, errors.New("unable to get client config")
	}

	client, err := networking.GetDefaultHTTPClient(config_obj, "", nil)
	if err != nil {
		return nil, err
	}

	if skipVerify {
		if err := networking.EnableSkipVerifyHttp(client, config_obj); err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequest(http.MethodPut, parsedUrl.String(), reader)
	if err != nil {
		return &uploads.UploadResponse{
			Error: err.Error(),
		}, err
	}

	req.ContentLength = contentLength
	req.SetBasicAuth(basicAuthUser, basicAuthPassword)

	resp, err := client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}

	if err != nil {
		return &uploads.UploadResponse{
			Error: err.Error(),
		}, err
	}

	scope.Log("upload_webdav: HTTP status %v", resp.StatusCode)

	return &uploads.UploadResponse{
		Path: name,
		Size: uint64(contentLength),
	}, nil
}

func (self WebDAVUploadFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "upload_webdav",
		Doc:     "Upload files to a WebDAV server.",
		ArgType: type_map.AddType(scope, &WebDAVUploadArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&WebDAVUploadFunction{})
}
