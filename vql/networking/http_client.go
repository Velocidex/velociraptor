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
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/crypto"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

var (
	mu sync.Mutex

	proxyHandler                     = http.ProxyFromEnvironment
	EmptyCookieJar *ordereddict.Dict = nil
)

const (
	HTTP_TAG       = "$http_client_cache"
	COOKIE_JAR_TAG = "$http_client_cookie_jar"
)

// Cache http clients in the scope to allow reuse.
type HTTPClientCache struct {
	mu    sync.Mutex
	cache map[string]*http.Client
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
	DisableSSLSecurity bool              `vfilter:"optional,field=disable_ssl_security,doc=Disable ssl certificate verifications."`
	TempfileExtension  string            `vfilter:"optional,field=tempfile_extension,doc=If specified we write to a tempfile. The content field will contain the full path to the tempfile."`
	RemoveLast         bool              `vfilter:"optional,field=remove_last,doc=If set we delay removal as much as possible."`
	RootCerts          string            `vfilter:"optional,field=root_ca,doc=As a better alternative to disable_ssl_security, allows root ca certs to be added here."`
	CookieJar          *ordereddict.Dict `vfilter:"optional,field=cookie_jar,doc=A cookie jar to use if provided. This is a dict of cookie structures."`
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
	config_obj *config_proto.ClientConfig,
	scope vfilter.Scope,
	arg *HttpPluginRequest) (*http.Client, error) {

	cache, pres := vql_subsystem.CacheGet(scope, HTTP_TAG).(*HTTPClientCache)
	if !pres {
		cache = &HTTPClientCache{cache: make(map[string]*http.Client)}
	}
	defer vql_subsystem.CacheSet(scope, HTTP_TAG, cache)

	return cache.GetHttpClient(config_obj, arg)
}

func (self *HTTPClientCache) GetHttpClient(
	config_obj *config_proto.ClientConfig,
	arg *HttpPluginRequest) (*http.Client, error) {

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

		result = &http.Client{
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
		}
		self.cache[key] = result
		return result, nil
	}

	// Create a http client without TLS security - this is sometimes
	// needed to access self signed servers. Ideally we should
	// add extra ca certs in arg.RootCerts.
	if arg.DisableSSLSecurity {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = proxyHandler
		transport.MaxIdleConns = 10
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}

		result = &http.Client{
			Timeout:   time.Second * 10000,
			Jar:       NewDictJar(arg.CookieJar),
			Transport: transport,
		}
		self.cache[key] = result
		return result, nil
	}

	result, err = GetDefaultHTTPClient(
		config_obj, arg.RootCerts, arg.CookieJar)
	if err != nil {
		return nil, err
	}

	self.cache[key] = result
	return result, nil
}

// If we deployed Velociraptor using self signed certificates we want
// to be able to trust our own server. Our own server is signed by our
// own CA and also may have a different common name (not related to
// DNS). For example, in self signed mode, the server certificate is
// signed for VelociraptorServer but may be served over
// "localhost". Using the default TLS configuration this connection
// will be rejected.

// Therefore in the special case where the server cert is signed by
// our own CA, and the Subject name is the pinned server name
// (VelociraptorServer), we do not need to compare the server's common
// name with the url.

// This function is based on
// https://go.dev/src/crypto/tls/handshake_client.go::verifyServerCertificate
func customVerifyConnection(
	CA_Pool *x509.CertPool,
	config_obj *config_proto.ClientConfig) func(conn tls.ConnectionState) error {

	// Check if the cert was signed by the Velociraptor CA
	private_opts := x509.VerifyOptions{
		CurrentTime:   time.Now(),
		Intermediates: x509.NewCertPool(),
		Roots:         x509.NewCertPool(),
	}
	if config_obj != nil {
		private_opts.Roots.AppendCertsFromPEM([]byte(config_obj.CaCertificate))
	}

	return func(conn tls.ConnectionState) error {
		// Used to verify certs using public roots
		public_opts := x509.VerifyOptions{
			CurrentTime:   time.Now(),
			Intermediates: x509.NewCertPool(),
			DNSName:       conn.ServerName,
			Roots:         CA_Pool,
		}

		// First parse all the server certs so we can verify them. The
		// server presents its main cert first, then any following
		// intermediates.
		var server_cert *x509.Certificate

		for i, cert := range conn.PeerCertificates {
			// First cert is server cert.
			if i == 0 {
				server_cert = cert

				// Velociraptor does not allow intermediates so this
				// should be sufficient to verify that the
				// Velociraptor CA signed it.
				_, err := server_cert.Verify(private_opts)
				if err == nil {
					// The Velociraptor CA signed it - we disregard
					// the DNS name and allow it.
					return nil
				}

			} else {
				public_opts.Intermediates.AddCert(cert)
			}
		}

		if server_cert == nil {
			return errors.New("Unknown server cert")
		}

		// Perform normal verification.
		_, err := server_cert.Verify(public_opts)
		return err
	}
}

func GetDefaultHTTPClient(
	config_obj *config_proto.ClientConfig,
	extra_roots string,
	cookie_jar *ordereddict.Dict) (*http.Client, error) {

	CA_Pool := x509.NewCertPool()
	if config_obj != nil {
		err := crypto.AddDefaultCerts(config_obj, CA_Pool)
		if err != nil {
			return nil, err
		}
	}

	// Allow access to public servers.
	crypto.AddPublicRoots(CA_Pool)

	if extra_roots != "" {
		if !CA_Pool.AppendCertsFromPEM([]byte(extra_roots)) {
			return nil, errors.New("Unable to parse root CA")
		}
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		MinVersion:         tls.VersionTLS12,
		ClientSessionCache: tls.NewLRUClientSessionCache(100),
		RootCAs:            CA_Pool,

		// Not actually skipping, we check the
		// cert in VerifyPeerCertificate
		InsecureSkipVerify: true,
		NextProtos:         []string{"http/1.1"},
		VerifyConnection:   customVerifyConnection(CA_Pool, config_obj),
	}
	transport.Proxy = proxyHandler
	transport.Dial = (&net.Dialer{
		KeepAlive: 600 * time.Second,
	}).Dial
	transport.MaxIdleConnsPerHost = 10
	transport.MaxIdleConns = 10

	result := &http.Client{
		Timeout:   time.Second * 10000,
		Jar:       NewDictJar(cookie_jar),
		Transport: transport,
	}

	return result, nil
}

func encodeParams(arg *HttpPluginRequest, scope vfilter.Scope) *url.Values {
	data := url.Values{}
	if arg.Params != nil {
		for _, member := range scope.GetMembers(arg.Params) {
			value, pres := scope.Associative(arg.Params, member)
			if pres {
				slice := reflect.ValueOf(value)
				if slice.Type().Kind() == reflect.Slice {
					for i := 0; i < slice.Len(); i++ {
						value := slice.Index(i).Interface()
						item, ok := value.(string)
						if ok {
							data.Add(member, item)
							continue
						}
					}
				}
				switch value.(type) {
				case vfilter.Null, *vfilter.Null:
					continue
				default:
					data.Add(member, fmt.Sprintf("%v", value))
				}
			}
		}
	}
	return &data
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
		client, err := GetHttpClient(config_obj, scope, arg)
		if err != nil {
			scope.Log("http_client: %v", err)
			return
		}

		var req *http.Request
		params := encodeParams(arg, scope)
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
				if arg.Data == "" && len(*params) != 0 {
					arg.Data = params.Encode()
				} else if arg.Data != "" && len(*params) != 0 {
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

		req.Header.Set("User-Agent", constants.USER_AGENT)

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

			tmpfile, err := ioutil.TempFile("", "tmp*"+arg.TempfileExtension)
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
				select {
				case <-ctx.Done():
					return
				case output_chan <- response:
				}
			}

			if err == io.EOF {
				break
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
		Name:    self.Name(),
		Doc:     "Make a http request.",
		ArgType: type_map.AddType(scope, &HttpPluginRequest{}),
		Version: 2,
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

func init() {
	vql_subsystem.RegisterPlugin(&_HttpPlugin{})
}
