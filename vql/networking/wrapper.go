package networking

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/smb"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/faults"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

var (
	unsupportedMethod = errors.New("Unsupported method for protocol")
	usernameRequired  = errors.New("Username and password required for SMB URLs")
	invalidSMBPath    = errors.New("Invalid SMB Path")
)

// Create a HTTPClient with superpowers to be used everywhere.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type fdWrapper struct {
	io.ReadCloser
	closer func()
}

func (self *fdWrapper) Close() error {
	err := self.ReadCloser.Close()
	self.closer()
	return err
}

type httpClientWrapper struct {
	http.Client
	scope vfilter.Scope
	ctx   context.Context
}

func GetHTTPClient(client HTTPClient) (*http.Client, error) {
	wrapper, ok := client.(*httpClientWrapper)
	if !ok {
		return nil, utils.Wrap(utils.InvalidArgError, "HTTPClient is not a wrapper")
	}
	return &wrapper.Client, nil
}

func (self httpClientWrapper) Do(req *http.Request) (*http.Response, error) {
	// Emulate a significant network delay on HTTP
	defer faults.FaultInjector.BlockHTTPDo(req.Context())

	if req.URL != nil {
		// Handle different url schemes
		switch req.URL.Scheme {
		case "smb":
			return self.doSMB(req)

		case "file":
			return self.doFile(req)

		case "unix":
			return self.doUnix(req)
		}
	}
	return self.Client.Do(req)
}

// Handle Unix URLs. The client is already created and connected to
// the socket file, but the req contains the Path that should be
// retrieved from the socket using http.
func (self httpClientWrapper) doUnix(req *http.Request) (*http.Response, error) {

	new_req := *req
	new_req.URL = &url.URL{
		Scheme: "http",
		Host:   "unix", // Does not matter as transport is already established in the client.
		Path:   req.URL.Path,
	}

	return self.Client.Do(&new_req)
}

// Use the file accessor to access file urls.
func (self httpClientWrapper) doFile(
	req *http.Request) (*http.Response, error) {
	if req.Method != "GET" {
		return nil, unsupportedMethod
	}

	accessor, err := accessors.GetAccessor(req.URL.Scheme, self.scope)
	if err != nil {
		return nil, err
	}

	file, err := accessor.Open(req.URL.Path)
	if err != nil {
		return nil, err
	}

	return &http.Response{
		Status:        "200 OK",
		StatusCode:    200,
		Body:          file,
		ContentLength: -1,
		Close:         true,
		Request:       req,
	}, nil
}

func (self httpClientWrapper) doSMB(
	req *http.Request) (*http.Response, error) {
	if req.Method != "GET" {
		return nil, unsupportedMethod
	}

	username := req.URL.User.Username()
	password, _ := req.URL.User.Password()
	hostname := req.URL.Hostname()

	if username == "" || password == "" {
		return nil, usernameRequired
	}

	components := utils.SplitComponents(req.URL.Path)
	if len(components) < 2 {
		return nil, invalidSMBPath
	}

	share := components[0]
	file_path := strings.Join(components[1:], "\\")

	cache, pres := vql_subsystem.CacheGet(
		self.scope, smb.SMB_TAG).(*smb.SMBMountCache)
	if !pres {
		sub_scope := self.scope.Copy()
		sub_scope.AppendVars(ordereddict.NewDict().
			Set("SMB_CREDENTIALS", ordereddict.NewDict().
				Set(hostname, fmt.Sprintf("%s:%s", username, password))))

		cache = smb.NewSMBMountCache(sub_scope)
		vql_subsystem.CacheSet(self.scope, smb.SMB_TAG, cache)
	}

	connection, closer, err := cache.GetHandle(hostname)
	if err != nil {
		return nil, err
	}

	fs, err := connection.Session().Mount(share)
	if err != nil {
		closer()
		return nil, err
	}

	fd, err := fs.Open(file_path)
	if err != nil {
		closer()
		return nil, err
	}

	return &http.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Body: &fdWrapper{
			ReadCloser: fd,
			closer:     closer,
		},
		ContentLength: -1,
		Close:         true,
		Request:       req,
	}, nil
}
