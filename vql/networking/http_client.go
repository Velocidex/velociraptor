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
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

type _HttpPluginRequest struct {
	Url     string      `vfilter:"required,field=url"`
	Params  vfilter.Any `vfilter:"optional,field=params"`
	Headers vfilter.Any `vfilter:"optional,field=headers"`
	Method  string      `vfilter:"optional,field=method"`
	Chunk   int         `vfilter:"optional,field=chunk_size"`

	// Sometimes it is useful to be able to query misconfigured hosts.
	DisableSSLSecurity bool `vfilter:"optional,field=disable_ssl_security"`
}

type _HttpPluginResponse struct {
	Url      string
	Content  string
	Response int
}

type _HttpPlugin struct{}

func customVerifyPeerCert(
	config_obj *api_proto.ClientConfig,
	url_str string,
	rawCerts [][]byte,
	verifiedChains [][]*x509.Certificate) error {

	url, err := url.Parse(url_str)
	if err != nil {
		return err
	}

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
		_, err = certs[0].Verify(*opts)
		return err
	}

	opts := &x509.VerifyOptions{
		CurrentTime:   time.Now(),
		Intermediates: x509.NewCertPool(),
	}

	// First check if the certs come from our CA - ignore the name
	// in that case.
	if config_obj != nil {
		opts.Roots = x509.NewCertPool()
		opts.Roots.AppendCertsFromPEM([]byte(config_obj.CaCertificate))

		// Yep its one of ours, just trust it.
		if verify_certs(opts) == nil {
			return nil
		}
	}

	// It is not signed by our CA - check the system store
	// and this time verify the Hostname.
	rootCAs, err := x509.SystemCertPool()
	if err != nil {
		return err
	}
	opts.DNSName = url.Hostname()
	opts.Roots = rootCAs

	return verify_certs(opts)
}

func getHttpClient(
	config_obj *api_proto.ClientConfig,
	arg *_HttpPluginRequest) *http.Client {

	// If we deployed Velociraptor using self signed certificates
	// we want to be able to trust our own server. Our own server
	// is signed by our own CA and also may have a different
	// common name (not related to DNS). Therefore in the special
	// case where the server cert is signed by our own CA we can
	// ignore the server's Common Name.

	result := &http.Client{}
	// It is a unix domain socket.
	if strings.HasPrefix(arg.Url, "/") {
		components := strings.Split(arg.Url, ":")
		if len(components) == 1 {
			components = append(components, "/")
		}
		result.Transport = &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", components[0])
			},
		}
		arg.Url = "http://unix" + components[1]

	} else {
		result.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // Not actually skipping, we check the cert in VerifyPeerCertificate
				VerifyPeerCertificate: func(
					rawCerts [][]byte,
					verifiedChains [][]*x509.Certificate) error {
					if arg.DisableSSLSecurity {
						return nil
					}
					return customVerifyPeerCert(
						config_obj,
						arg.Url,
						rawCerts,
						verifiedChains)
				},
			},
		}
	}

	return result
}

func encodeParams(arg *_HttpPluginRequest, scope *vfilter.Scope) *url.Values {
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
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	arg := &_HttpPluginRequest{}
	err := vfilter.ExtractArgs(scope, args, arg)
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

		any_config_obj, _ := scope.Resolve("config")
		config_obj := any_config_obj.(*api_proto.ClientConfig)

		params := encodeParams(arg, scope)
		client := getHttpClient(config_obj, arg)
		req, err := http.NewRequest(
			arg.Method, arg.Url,
			strings.NewReader(params.Encode()))
		if err != nil {
			scope.Log("%s: %s", self.Name(), err.Error())
			return
		}

		http_resp, err := client.Do(req)
		if err != nil {
			output_chan <- &_HttpPluginResponse{
				Url:      arg.Url,
				Response: 500,
				Content:  err.Error(),
			}
			return
		}
		defer http_resp.Body.Close()

		response := &_HttpPluginResponse{
			Url:      arg.Url,
			Response: http_resp.StatusCode,
		}

		buf := make([]byte, arg.Chunk)
		for {
			n, err := io.ReadFull(http_resp.Body, buf)
			if n > 0 {
				response.Content = string(buf[:n])
				output_chan <- response
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

func (self _HttpPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    self.Name(),
		Doc:     "Make a http request.",
		RowType: type_map.AddType(scope, &_HttpPluginResponse{}),
		ArgType: type_map.AddType(scope, &_HttpPluginRequest{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&_HttpPlugin{})
}
