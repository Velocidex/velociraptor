//+build extras

package tools

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/glob"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type WebDAVUploadArgs struct {
	File              string `vfilter:"required,field=file,doc=The file to upload"`
	Name              string `vfilter:"optional,field=name,doc=The name that the file should have on the server"`
	Accessor          string `vfilter:"optional,field=accessor,doc=The accessor to use"`
	Url               string `vfilter:"required,field=url,doc=The WebDAV url"`
	BasicAuthUser     string `vfilter:"optional,field=basic_auth_user,doc=The username to use in HTTP basic auth"`
	BasicAuthPassword string `vfilter:"optional,field=basic_auth_password,doc=The password to use in HTTP basic auth"`
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

	err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
	if err != nil {
		scope.Log("upload_webdav: %s", err)
		return vfilter.Null{}
	}

	accessor, err := glob.GetAccessor(arg.Accessor, scope)
	if err != nil {
		scope.Log("upload_webdav: %v", err)
		return vfilter.Null{}
	}

	file, err := accessor.Open(arg.File)
	if err != nil {
		scope.Log("upload_webdav: Unable to open %s: %s",
			arg.File, err.Error())
		return &vfilter.Null{}
	}
	defer file.Close()

	if arg.Name == "" {
		arg.Name = arg.File
	}

	stat, err := file.Stat()
	if err != nil {
		scope.Log("upload_webdav: Unable to stat %s: %v",
			arg.File, err)
	} else if !stat.IsDir() {
		// Abort uploading when the scope is destroyed.
		sub_ctx, cancel := context.WithCancel(ctx)
		_ = scope.AddDestructor(cancel)

		upload_response, err := upload_webdav(
			sub_ctx, scope, file, stat.Size(),
			arg.Name, arg.Url, arg.BasicAuthUser, arg.BasicAuthPassword)
		if err != nil {
			scope.Log("upload_webdav: %v", err)
			return vfilter.Null{}
		}
		return upload_response
	}

	return vfilter.Null{}
}

func upload_webdav(ctx context.Context, scope vfilter.Scope,
	reader io.Reader, contentLength int64,
	name string, webdavUrl string,
	basicAuthUser string, basicAuthPassword string) (
	*api.UploadResponse, error) {

	scope.Log("upload_webdav: Uploading %v to %v", name, webdavUrl)

	parsedUrl, err := url.Parse(webdavUrl)
	if err != nil {
		return &api.UploadResponse{
			Error: err.Error(),
		}, err
	}
	parsedUrl.Path = path.Join(parsedUrl.Path, name)

	var netTransport = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout: 30 * time.Second, // TCP connect timeout
		}).DialContext,
		TLSHandshakeTimeout: 30 * time.Second,
	}
	client := &http.Client{
		Transport: netTransport,
	}

	req, err := http.NewRequest(http.MethodPut, parsedUrl.String(), reader)
	if err != nil {
		return &api.UploadResponse{
			Error: err.Error(),
		}, err
	}

	req.ContentLength = contentLength
	req.SetBasicAuth(basicAuthUser, basicAuthPassword)

	resp, err := client.Do(req)
	if err != nil {
		return &api.UploadResponse{
			Error: err.Error(),
		}, err
	}

	scope.Log("upload_webdav: HTTP status %v", resp.StatusCode)

	return &api.UploadResponse{
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
