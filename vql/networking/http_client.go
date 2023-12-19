/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2022 Rapid7 Inc.

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
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

var (
	mu sync.Mutex

	proxyHandler                     = http.ProxyFromEnvironment
	EmptyCookieJar *ordereddict.Dict = nil

	errSkipVerifyDenied = errors.New("SkipVerify not allowed due to TLS certificate verification policy")
)

const (
	HTTP_TAG       = "$http_client_cache"
	COOKIE_JAR_TAG = "$http_client_cookie_jar"
)

// Cache http clients in the scope to allow reuse.
type HTTPClientCache struct {
	mu    sync.Mutex
	cache map[string]HTTPClient
}

func (self *HTTPClientCache) getCacheKey(url *url.URL) string {
	return url.Scheme + ":" + url.Hostname()
}

type HttpPluginRequest struct {
	Url     string      `vfilter:"required,field=url,doc=The URL to fetch"`
	Params  vfilter.Any `vfilter:"optional,field=params,doc=Parameters to encode as POST or GET query strings"`
	Headers vfilter.Any `vfilter:"optional,field=headers,doc=A dict of headers to send."`
	Method  string      `vfilter:"optional,field=method,doc=HTTP method to use (GET, POST, PUT, PATCH, DELETE)"`
	Data    string      `vfilter:"optional,field=data,doc=If specified we write this raw data into a POST request instead of encoding the params above."`
	Chunk   int         `vfilter:"optional,field=chunk_size,doc=Read input with this chunk size and send each chunk as a row"`

	// Sometimes it is useful to be able to query misconfigured hosts.
	DisableSSLSecurity bool              `vfilter:"optional,field=disable_ssl_security,doc=Disable ssl certificate verifications (deprecated in favor of SkipVerify)."`
	SkipVerify         bool              `vfilter:"optional,field=skip_verify,doc=Disable ssl certificate verifications."`
	TempfileExtension  string            `vfilter:"optional,field=tempfile_extension,doc=If specified we write to a tempfile. The content field will contain the full path to the tempfile."`
	RemoveLast         bool              `vfilter:"optional,field=remove_last,doc=If set we delay removal as much as possible."`
	RootCerts          string            `vfilter:"optional,field=root_ca,doc=As a better alternative to disable_ssl_security, allows root ca certs to be added here."`
	CookieJar          *ordereddict.Dict `vfilter:"optional,field=cookie_jar,doc=A cookie jar to use if provided. This is a dict of cookie structures."`
	UserAgent          string            `vfilter:"optional,field=user_agent,doc=If specified, set a HTTP User-Agent."`
}

type _HttpPluginResponse struct {
	Url      string
	Content  string
	Response int
	Headers  vfilter.Any
}

type _HttpPlugin struct{}

// Get a potentially cached http client.
func GetHttpClient(
	ctx context.Context,
	config_obj *config_proto.ClientConfig,
	scope vfilter.Scope,
	arg *HttpPluginRequest) (HTTPClient, error) {

	cache, pres := vql_subsystem.CacheGet(scope, HTTP_TAG).(*HTTPClientCache)
	if !pres {
		cache = &HTTPClientCache{cache: make(map[string]HTTPClient)}
	}
	defer vql_subsystem.CacheSet(scope, HTTP_TAG, cache)

	return cache.GetHttpClient(ctx, config_obj, arg, scope)
}

func (self *HTTPClientCache) GetHttpClient(
	ctx context.Context,
	config_obj *config_proto.ClientConfig,
	arg *HttpPluginRequest,
	scope vfilter.Scope) (HTTPClient, error) {

	self.mu.Lock()
	defer self.mu.Unlock()

	// Check the cache for an existing http client.
	url_obj, err := url.Parse(arg.Url)
	if err != nil {
		return nil, err
	}

	key := self.getCacheKey(url_obj)
	result, pres := self.cache[key]
	if pres {
		return result, nil
	}

	// Allow a unix path to be interpreted as simply a http over
	// unix domain socket (used by e.g. docker)
	if strings.HasPrefix(arg.Url, "/") {
		components := strings.Split(arg.Url, ":")
		if len(components) == 1 {
			components = append(components, "/")
		}
		arg.Url = "http://unix" + components[1]

		result = &httpClientWrapper{
			Client: http.Client{
				Timeout: time.Second * 10000,
				Transport: &http.Transport{
					MaxIdleConnsPerHost: 10,
					DialContext: func(_ context.Context, _, _ string) (
						net.Conn, error) {
						return net.Dial("unix", components[0])
					},
					TLSNextProto: make(map[string]func(
						authority string, c *tls.Conn) http.RoundTripper),
				},
			},
			ctx:   ctx,
			scope: scope,
		}
		self.cache[key] = result
		return result, nil
	}

	// Create a http client without TLS security - this is sometimes
	// needed to access self signed servers. Ideally we should
	// add extra ca certs in arg.RootCerts.
	if arg.DisableSSLSecurity || arg.SkipVerify {
		if arg.DisableSSLSecurity {
			scope.Log("http_client: DisableSSLSecurity is deprecated, please use SkipVerify instead")
		}

		transport, err := GetHttpTransport(config_obj, "")
		if err != nil {
			return nil, err
		}

		if err = EnableSkipVerify(transport.TLSClientConfig, config_obj); err != nil {
			return nil, err
		}

		result = &httpClientWrapper{
			Client: http.Client{
				Timeout:   time.Second * 10000,
				Jar:       NewDictJar(arg.CookieJar),
				Transport: transport,
			},
			ctx:   ctx,
			scope: scope,
		}
		self.cache[key] = result
		return result, nil
	}

	result, err = GetDefaultHTTPClient(ctx,
		config_obj, scope, arg.RootCerts, arg.CookieJar)
	if err != nil {
		return nil, err
	}

	self.cache[key] = result
	return result, nil
}

func GetDefaultHTTPClient(
	ctx context.Context,
	config_obj *config_proto.ClientConfig,
	scope vfilter.Scope,
	extra_roots string,
	cookie_jar *ordereddict.Dict) (HTTPClient, error) {

	transport, err := GetHttpTransport(config_obj, extra_roots)
	if err != nil {
		return nil, err
	}

	return &httpClientWrapper{
		Client: http.Client{
			Timeout:   time.Second * 10000,
			Jar:       NewDictJar(cookie_jar),
			Transport: transport,
		},
		ctx:   ctx,
		scope: scope,
	}, nil
}

func (self *_HttpPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	arg := &HttpPluginRequest{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		goto error
	}

	if arg.Chunk == 0 {
		arg.Chunk = 4 * 1024 * 1024
	}

	if arg.Method == "" {
		arg.Method = "GET"
	}

	go func() {
		defer close(output_chan)

		err := vql_subsystem.CheckAccess(scope, acls.COLLECT_SERVER)
		if err != nil {
			scope.Log("http_client: %s", err)
			return
		}

		// If the user did not provide a cookie jar we use one for the
		// session.
		var ok bool

		if utils.IsNil(arg.CookieJar) {
			arg.CookieJar, ok = vql_subsystem.CacheGet(
				scope, COOKIE_JAR_TAG).(*ordereddict.Dict)
			if !ok {
				arg.CookieJar = ordereddict.NewDict()
				vql_subsystem.CacheSet(scope, COOKIE_JAR_TAG, arg.CookieJar)
			}
		}

		config_obj, _ := artifacts.GetConfig(scope)
		client, err := GetHttpClient(ctx, config_obj, scope, arg)
		if err != nil {
			scope.Log("http_client: %v", err)
			return
		}

		var req *http.Request
		params := functions.EncodeParams(arg.Params, scope)
		switch method := strings.ToUpper(arg.Method); method {
		case "GET":
			{
				req, err = http.NewRequestWithContext(
					ctx, method, arg.Url, strings.NewReader(arg.Data))
				if err != nil {
					scope.Log("%s: %v", self.Name(), err)
					return
				}
				req.URL.RawQuery = params.Encode()
			}
		case "POST", "PUT", "PATCH", "DELETE":
			{
				// Set body to params if arg.Data is empty
				if arg.Data == "" && len(params) != 0 {
					arg.Data = params.Encode()
				} else if arg.Data != "" && len(params) != 0 {
					// Shouldn't set both params and data. Warn user
					scope.Log("http_client: Both params and data set. Defaulting to data.")
				}
				req, err = http.NewRequestWithContext(
					ctx, method, arg.Url, strings.NewReader(arg.Data))
				if err != nil {
					scope.Log("%s: %v", self.Name(), err)
					return
				}
			}
		default:
			{
				scope.Log("http_client: Invalid HTTP Method %s", method)
				return
			}
		}

		scope.Log("Fetching %v\n", arg.Url)
		if arg.UserAgent == "" {
			arg.UserAgent = constants.USER_AGENT
		}

		req.Header.Set("User-Agent", arg.UserAgent)

		// Set various headers
		if arg.Headers != nil {
			for _, member := range scope.GetMembers(arg.Headers) {
				value, pres := scope.Associative(arg.Headers, member)
				if pres {
					lazy_v, ok := value.(types.LazyExpr)
					if ok {
						value = lazy_v.Reduce(ctx)
					}

					str_value, ok := value.(string)
					if ok {
						req.Header.Set(member, str_value)
					}
				}
			}
		}

		http_resp, err := client.Do(req)
		if http_resp != nil {
			defer http_resp.Body.Close()
		}

		if err != nil {
			scope.Log("http_client: Error %v while fetching %v",
				err, arg.Url)
			select {
			case <-ctx.Done():
				return
			case output_chan <- &_HttpPluginResponse{
				Url:      arg.Url,
				Response: 500,
				Content:  err.Error()}:
			}
			return
		}

		response := &_HttpPluginResponse{
			Url:      arg.Url,
			Response: http_resp.StatusCode,
			Headers:  http_resp.Header,
		}

		if arg.TempfileExtension != "" {
			tmpfile, err := os.CreateTemp("", "tmp*"+arg.TempfileExtension)
			if err != nil {
				scope.Log("http_client: %v", err)
				return
			}

			remove := func() {
				remove_tmpfile(tmpfile.Name(), scope)
			}
			if arg.RemoveLast {
				scope.Log("Adding global destructor for %v", tmpfile.Name())
				err := vql_subsystem.GetRootScope(scope).AddDestructor(remove)
				if err != nil {
					remove()
					scope.Log("http_client: %v", err)
					return
				}
			} else {
				err := scope.AddDestructor(remove)
				if err != nil {
					remove()
					scope.Log("http_client: %v", err)
					return
				}
			}

			scope.Log("http_client: Downloading %v into %v",
				arg.Url, tmpfile.Name())

			response.Content = tmpfile.Name()
			_, err = utils.Copy(ctx, tmpfile, http_resp.Body)
			if err != nil && err != io.EOF {
				scope.Log("http_client: Reading error %v", err)
			}

			// Force the file to be closed *before* we
			// emit it to the VQL engine.
			tmpfile.Close()

			select {
			case <-ctx.Done():
				return
			case output_chan <- response:
			}

			return
		}

		buf := make([]byte, arg.Chunk)
		for {
			n, err := io.ReadFull(http_resp.Body, buf)
			if n > 0 {
				response.Content = string(buf[:n])
			} else if err == io.EOF {
				response.Content = ""
			} else if err != nil {
				break
			}

			select {
			case <-ctx.Done():
				return
			case output_chan <- response:
			}

			if err != nil {
				break
			}
		}
	}()

	return output_chan

error:
	scope.Log("%s: %s", self.Name(), err.Error())
	close(output_chan)
	return output_chan
}

func (self _HttpPlugin) Name() string {
	return "http_client"
}

func (self _HttpPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     self.Name(),
		Doc:      "Make a http request.",
		ArgType:  type_map.AddType(scope, &HttpPluginRequest{}),
		Version:  2,
		Metadata: vql.VQLMetadata().Permissions(acls.COLLECT_SERVER).Build(),
	}
}

// Make sure the file is removed when the query is done.
func remove_tmpfile(tmpfile string, scope vfilter.Scope) {
	scope.Log("tempfile: removing tempfile %v", tmpfile)

	// On windows especially we can not remove files that
	// are opened by something else, so we keep trying for
	// a while.
	for i := 0; i < 100; i++ {
		err := os.Remove(tmpfile)
		if err == nil {
			break
		}
		scope.Log("tempfile: Error %v - will retry", err)
		time.Sleep(time.Second)
	}
}

func SetProxy(handler func(*http.Request) (*url.URL, error)) {
	mu.Lock()
	defer mu.Unlock()

	proxyHandler = handler
}

func GetProxy() func(*http.Request) (*url.URL, error) {
	mu.Lock()
	defer mu.Unlock()

	return proxyHandler
}

// If the TLS Verification policy allows it, enable SkipVerify to
// allow connections to invalid TLS servers.
func EnableSkipVerifyHttp(client HTTPClient, config_obj *config_proto.ClientConfig) error {
	http_client := client.(*httpClientWrapper)

	if http_client == nil || http_client.Transport == nil {
		return nil
	}

	t, ok := http_client.Transport.(*http.Transport)
	if !ok {
		return errors.New("http client does not have a compatible transport")
	}

	return EnableSkipVerify(t.TLSClientConfig, config_obj)
}

func init() {
	vql_subsystem.RegisterPlugin(&_HttpPlugin{})
}
