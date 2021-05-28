/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

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
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

var (
	mu sync.Mutex

	// HTTP clients can be reused between goroutines. This should
	// keep TCP connections up etc.
	http_client        *http.Client
	http_client_no_ssl *http.Client
)

type HttpPluginRequest struct {
	Url     string      `vfilter:"required,field=url,doc=The URL to fetch"`
	Params  vfilter.Any `vfilter:"optional,field=params,doc=Parameters to encode as POST or GET query strings"`
	Headers vfilter.Any `vfilter:"optional,field=headers,doc=A dict of headers to send."`
	Method  string      `vfilter:"optional,field=method,doc=HTTP method to use (GET, POST)"`
	Data    string      `vfilter:"optional,field=data,doc=If specified we write this raw data into a POST request instead of encoding the params above."`
	Chunk   int         `vfilter:"optional,field=chunk_size,doc=Read input with this chunk size and send each chunk as a row"`

	// Sometimes it is useful to be able to query misconfigured hosts.
	DisableSSLSecurity bool   `vfilter:"optional,field=disable_ssl_security,doc=Disable ssl certificate verifications."`
	TempfileExtension  string `vfilter:"optional,field=tempfile_extension,doc=If specified we write to a tempfile. The content field will contain the full path to the tempfile."`
	RemoveLast         bool   `vfilter:"optional,field=remove_last,doc=If set we delay removal as much as possible."`
}

type _HttpPluginResponse struct {
	Url      string
	Content  string
	Response int
}

type _HttpPlugin struct{}

func customVerifyPeerCert(
	config_obj *config_proto.ClientConfig,
	rawCerts [][]byte,
	verifiedChains [][]*x509.Certificate) error {

	certs := make([]*x509.Certificate, len(rawCerts))
	for i, rawCert := range rawCerts {
		cert, err := x509.ParseCertificate(rawCert)
		if err != nil {
			return err
		}
		certs[i] = cert
	}

	verify_certs := func(opts *x509.VerifyOptions) error {
		for i, cert := range certs {
			if i == 0 {
				continue
			}
			opts.Intermediates.AddCert(cert)
		}
		_, err := certs[0].Verify(*opts)
		return err
	}

	// First check if the certs come from our CA - ignore the name
	// in that case.
	if config_obj != nil {
		opts := &x509.VerifyOptions{
			CurrentTime:   time.Now(),
			Intermediates: x509.NewCertPool(),
			Roots:         x509.NewCertPool(),
		}
		opts.Roots.AppendCertsFromPEM([]byte(config_obj.CaCertificate))

		// Yep its one of ours, just trust it.
		if verify_certs(opts) == nil {
			return nil
		}
	}

	// Perform normal verification.
	return verify_certs(&x509.VerifyOptions{
		CurrentTime:   time.Now(),
		Intermediates: x509.NewCertPool(),
	})
}

func GetHttpClient(
	config_obj *config_proto.ClientConfig,
	arg *HttpPluginRequest) *http.Client {

	// If we deployed Velociraptor using self signed certificates
	// we want to be able to trust our own server. Our own server
	// is signed by our own CA and also may have a different
	// common name (not related to DNS). Therefore in the special
	// case where the server cert is signed by our own CA we can
	// ignore the server's Common Name.

	// It is a unix domain socket.
	if arg != nil && strings.HasPrefix(arg.Url, "/") {
		components := strings.Split(arg.Url, ":")
		if len(components) == 1 {
			components = append(components, "/")
		}
		arg.Url = "http://unix" + components[1]

		return &http.Client{
			Timeout: time.Second * 10000,
			Transport: &http.Transport{
				Proxy:               http.ProxyFromEnvironment,
				MaxIdleConnsPerHost: 10,
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", components[0])
				},
			},
		}
	}

	mu.Lock()
	defer mu.Unlock()

	if arg != nil && arg.DisableSSLSecurity {
		if http_client_no_ssl != nil {
			return http_client_no_ssl
		}

		http_client_no_ssl = &http.Client{
			Timeout: time.Second * 10000,
			Transport: &http.Transport{
				Proxy:        http.ProxyFromEnvironment,
				MaxIdleConns: 10,
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}

		return http_client_no_ssl
	}

	if http_client != nil {
		return http_client
	}

	http_client = &http.Client{
		Timeout: time.Second * 10000,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			Dial: (&net.Dialer{
				KeepAlive: 600 * time.Second,
			}).Dial,
			MaxIdleConnsPerHost: 10,
			MaxIdleConns:        10,
			TLSClientConfig: &tls.Config{
				// Not actually skipping, we check the
				// cert in VerifyPeerCertificate
				InsecureSkipVerify: true,
				VerifyPeerCertificate: func(
					rawCerts [][]byte,
					verifiedChains [][]*x509.Certificate) error {
					return customVerifyPeerCert(
						config_obj,
						rawCerts,
						verifiedChains)
				},
			},
		},
	}

	return http_client
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

		config_obj, _ := artifacts.GetConfig(scope)

		params := encodeParams(arg, scope)
		client := GetHttpClient(config_obj, arg)

		data := arg.Data
		if data == "" {
			data = params.Encode()
		}

		req, err := http.NewRequestWithContext(
			ctx, arg.Method, arg.Url, strings.NewReader(data))
		if err != nil {
			scope.Log("%s: %s", self.Name(), err.Error())
			return
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
		utils.Debug(err)
		time.Sleep(time.Second)
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&_HttpPlugin{})
}
