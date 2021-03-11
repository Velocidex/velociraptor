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
package api

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"www.velocidex.com/golang/velociraptor/api/authenticators"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/crypto"
	file_store "www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/logging"
)

// A Mux for the reverse proxy feature.
func AddProxyMux(config_obj *config_proto.Config, mux *http.ServeMux) error {
	if config_obj.GUI == nil {
		return errors.New("GUI not configured")
	}

	logger := logging.GetLogger(config_obj, &logging.GUIComponent)

	for _, reverse_proxy_config := range config_obj.GUI.ReverseProxy {
		target, err := url.Parse(reverse_proxy_config.Url)
		if err != nil {
			return err
		}

		logger.Info("Adding reverse proxy router from %v to %v", reverse_proxy_config.Route,
			reverse_proxy_config.Url)

		var handler http.Handler
		if target.Scheme == "file" {
			handler = http.StripPrefix(reverse_proxy_config.Route,
				http.FileServer(http.Dir(target.Path)))

		} else {
			handler = http.StripPrefix(reverse_proxy_config.Route,
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					r.URL.Host = target.Host
					r.URL.Scheme = target.Scheme
					r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
					r.Host = target.Host

					// If we require auth we do
					// not pass the auth header to
					// the target of the
					// proxy. Otherwise we leave
					// authentication to it.
					if reverse_proxy_config.RequireAuth {
						r.Header.Del("Authorization")
					}

					httputil.NewSingleHostReverseProxy(target).ServeHTTP(w, r)
				}))
		}

		if reverse_proxy_config.RequireAuth {
			auther, err := authenticators.NewAuthenticator(config_obj)
			if err != nil {
				return err
			}
			handler = auther.AuthenticateUserHandler(config_obj, handler)
		}

		mux.Handle(reverse_proxy_config.Route, handler)
	}

	return nil
}

// Prepares a mux for the GUI by adding handlers required by the GUI.
func PrepareGUIMux(
	ctx context.Context,
	config_obj *config_proto.Config, mux *http.ServeMux) (http.Handler, error) {
	if config_obj.GUI == nil {
		return nil, errors.New("GUI not configured")
	}

	h, err := GetAPIHandler(ctx, config_obj)
	if err != nil {
		return nil, err
	}

	// The Authenticator is responsible for authenticating the
	// user via some method. Authenticators may install their own
	// mux handlers required for the various auth schemes but
	// ultimately they are responsible for checking the user is
	// properly authenticated.
	auther, err := authenticators.NewAuthenticator(config_obj)
	if err != nil {
		return nil, err
	}

	err = auther.AddHandlers(config_obj, mux)
	if err != nil {
		return nil, err
	}

	base := config_obj.GUI.BasePath

	mux.Handle(base+"/api/", csrfProtect(config_obj,
		auther.AuthenticateUserHandler(config_obj, h)))

	mux.Handle(base+"/api/v1/DownloadTable", csrfProtect(config_obj,
		auther.AuthenticateUserHandler(
			config_obj, downloadTable(config_obj))))

	mux.Handle(base+"/api/v1/DownloadVFSFile", csrfProtect(config_obj,
		auther.AuthenticateUserHandler(
			config_obj, vfsFileDownloadHandler(config_obj))))

	mux.Handle(base+"/api/v1/DownloadVFSFolder", csrfProtect(config_obj,
		auther.AuthenticateUserHandler(
			config_obj, vfsFolderDownloadHandler(config_obj))))

	mux.Handle(base+"/api/v1/UploadTool", csrfProtect(config_obj,
		auther.AuthenticateUserHandler(
			config_obj, toolUploadHandler(config_obj))))

	// Serve prepared zip files.
	mux.Handle(base+"/downloads/", csrfProtect(config_obj,
		auther.AuthenticateUserHandler(
			config_obj, http.FileServer(
				api.NewFileSystem(
					config_obj,
					file_store.GetFileStore(config_obj),
					"/downloads/")))))

	// Serve notebook items
	mux.Handle(base+"/notebooks/", csrfProtect(config_obj,
		auther.AuthenticateUserHandler(
			config_obj, http.FileServer(
				api.NewFileSystem(
					config_obj,
					file_store.GetFileStore(config_obj),
					"/notebooks/")))))

	// Assets etc do not need auth.
	install_static_assets(config_obj, mux)

	// Add reverse proxy support.
	err = AddProxyMux(config_obj, mux)
	if err != nil {
		return nil, err
	}

	h, err = GetTemplateHandler(config_obj, "/index.html")
	if err != nil {
		return nil, err
	}
	mux.Handle(base+"/app/index.html", csrfProtect(config_obj,
		auther.AuthenticateUserHandler(config_obj, h)))

	mux.Handle(base+"/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, base+"/app/index.html", 302)
	}))

	return mux, nil
}

type _templateArgs struct {
	Timestamp  int64
	Heading    string
	Help_url   string
	Report_url string
	Version    string
	CsrfToken  string
	BasePath   string
}

// An api handler which connects to the gRPC service (i.e. it is a
// gRPC client).
func GetAPIHandler(
	ctx context.Context,
	config_obj *config_proto.Config) (http.Handler, error) {

	if config_obj.Client == nil ||
		config_obj.GUI == nil ||
		config_obj.API == nil {
		return nil, errors.New("Client not configured")
	}

	// We need to tell when someone uses HEAD method on our grpc
	// proxy so we need to pass this information from the request
	// to the gRPC server using the gRPC metadata.
	grpc_proxy_mux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			MarshalOptions: protojson.MarshalOptions{
				UseProtoNames: true,
			},
			UnmarshalOptions: protojson.UnmarshalOptions{
				DiscardUnknown: true,
			},
		}),
		runtime.WithMetadata(
			func(ctx context.Context, req *http.Request) metadata.MD {
				md := map[string]string{
					"METHOD": req.Method,
				}
				username, ok := req.Context().Value(
					constants.GRPC_USER_CONTEXT).(string)
				if ok {
					md["USER"] = username
				}

				return metadata.New(md)
			}),
	)

	// We use a dedicated gw certificate. The gRPC server will
	// only accept a relayed username from us.
	cert, err := tls.X509KeyPair(
		[]byte(config_obj.GUI.GwCertificate),
		[]byte(config_obj.GUI.GwPrivateKey))
	if err != nil {
		return nil, err
	}

	// Authenticate API clients using certificates.
	CA_Pool := x509.NewCertPool()
	CA_Pool.AppendCertsFromPEM([]byte(config_obj.Client.CaCertificate))

	// Make sure the cert is ok.
	gw_cert, err := crypto.ParseX509CertFromPemStr(
		[]byte(config_obj.GUI.GwCertificate))
	if err != nil {
		return nil, err
	}

	_, err = gw_cert.Verify(x509.VerifyOptions{Roots: CA_Pool})
	if err != nil {
		return nil, err
	}

	gw_name := crypto.GetSubjectName(gw_cert)
	if gw_name != config_obj.API.PinnedGwName {
		return nil, errors.New("GUI gRPC proxy Certificate is not correct")
	}

	creds := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      CA_Pool,
		ServerName:   config_obj.Client.PinnedServerName,
	})

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
	}

	bind_addr := grpc_client.GetAPIConnectionString(config_obj)
	err = api_proto.RegisterAPIHandlerFromEndpoint(
		ctx, grpc_proxy_mux, bind_addr, opts)
	if err != nil {
		return nil, err
	}

	base := config_obj.GUI.BasePath

	reverse_proxy_mux := http.NewServeMux()
	reverse_proxy_mux.Handle(base+"/api/v1/",
		http.StripPrefix(base, grpc_proxy_mux))

	return reverse_proxy_mux, nil
}
