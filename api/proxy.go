/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

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
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"

	errors "github.com/go-errors/errors"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/api/authenticators"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	api_utils "www.velocidex.com/golang/velociraptor/api/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/logging"
	debug_server "www.velocidex.com/golang/velociraptor/services/debug/server"
	"www.velocidex.com/golang/velociraptor/utils"
)

// A Mux for the reverse proxy feature.
func AddProxyMux(config_obj *config_proto.Config, mux *api_utils.ServeMux) error {
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
			handler = api_utils.StripPrefix(reverse_proxy_config.Route,
				http.FileServer(http.Dir(target.Path)))

		} else {
			handler = api_utils.StripPrefix(reverse_proxy_config.Route,
				api_utils.HandlerFunc(nil,
					func(w http.ResponseWriter, r *http.Request) {
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
			// Minimum level of access should be READ_RESULTS
			handler = auther.AuthenticateUserHandler(handler, acls.READ_RESULTS)
		}

		mux.Handle(reverse_proxy_config.Route, handler)
	}

	return nil
}

// Prepares a mux for the GUI by adding handlers required by the GUI.
func PrepareGUIMux(
	ctx context.Context,
	config_obj *config_proto.Config,
	mux *api_utils.ServeMux) (http.Handler, error) {
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
	if config_obj.GUI != nil && config_obj.GUI.Authenticator != nil {
		logger := logging.GetLogger(config_obj, &logging.GUIComponent)
		logger.Info("GUI will use the %v authenticator", config_obj.GUI.Authenticator.Type)
	}

	// Add the authenticator specific handlers.
	err = auther.AddHandlers(mux)
	if err != nil {
		return nil, err
	}

	// Add the logout handlers
	err = auther.AddLogoff(mux)
	if err != nil {
		return nil, err
	}

	base_path := api_utils.GetBasePath(config_obj)

	mux.Handle(api_utils.GetBasePath(config_obj, "/api/"),
		ipFilter(config_obj,
			csrfProtect(config_obj,
				auther.AuthenticateUserHandler(h, acls.READ_RESULTS))))

	mux.Handle(api_utils.GetBasePath(config_obj, "/api/v1/DownloadTable"),
		ipFilter(config_obj, csrfProtect(config_obj,
			auther.AuthenticateUserHandler(
				downloadTable(), acls.READ_RESULTS))))

	mux.Handle(api_utils.GetBasePath(config_obj, "/api/v1/DownloadVFSFile"),
		ipFilter(config_obj, csrfProtect(config_obj,
			auther.AuthenticateUserHandler(
				vfsFileDownloadHandler(), acls.READ_RESULTS))))

	mux.Handle(api_utils.GetBasePath(config_obj, "/api/v1/UploadTool"),
		ipFilter(config_obj, csrfProtect(config_obj,
			auther.AuthenticateUserHandler(
				toolUploadHandler(), acls.READ_RESULTS))))

	mux.Handle(api_utils.GetBasePath(config_obj, "/api/v1/UploadFormFile"),
		ipFilter(config_obj, csrfProtect(config_obj,
			auther.AuthenticateUserHandler(
				formUploadHandler(), acls.READ_RESULTS))))

	// Serve prepared zip files.
	mux.Handle(api_utils.GetBasePath(config_obj, "/downloads/"),
		ipFilter(config_obj, csrfProtect(config_obj,
			auther.AuthenticateUserHandler(
				api_utils.StripPrefix(base_path,
					downloadFileStore([]string{"downloads"})),
				acls.READ_RESULTS))))

	// Serve notebook items
	mux.Handle(api_utils.GetBasePath(config_obj, "/notebooks/"),
		ipFilter(config_obj, csrfProtect(config_obj,
			auther.AuthenticateUserHandler(
				api_utils.StripPrefix(base_path,
					downloadFileStore([]string{"notebooks"})),
				acls.READ_RESULTS))))

	// Serve files from hunt notebooks
	mux.Handle(api_utils.GetBasePath(config_obj, "/hunts/"),
		ipFilter(config_obj, csrfProtect(config_obj,
			auther.AuthenticateUserHandler(
				api_utils.StripPrefix(base_path,
					downloadFileStore([]string{"hunts"})),
				acls.READ_RESULTS))))

	// Serve files from client notebooks
	mux.Handle(api_utils.GetBasePath(config_obj, "/clients/"),
		ipFilter(config_obj, csrfProtect(config_obj,
			auther.AuthenticateUserHandler(
				api_utils.StripPrefix(base_path,
					downloadFileStore([]string{"clients"})),
				acls.READ_RESULTS))))

	// Enable debug endpoints but only for users with SERVER_ADMIN on
	// the root org, because the debug server currently exposes all
	// org's data. The debug server requires access to the root org!
	mux.Handle(api_utils.GetBasePath(config_obj, "/debug/"),
		ipFilter(config_obj, csrfProtect(config_obj,
			auther.AuthenticateUserHandler(
				api_utils.StripPrefix(base_path,
					debug_server.DebugMux(config_obj, base_path).
						RequireRootOrg()),
				acls.SERVER_ADMIN))))

	// Assets etc do not need auth.
	install_static_assets(ctx, config_obj, mux)

	// Add reverse proxy support.
	err = AddProxyMux(config_obj, mux)
	if err != nil {
		return nil, err
	}

	h, err = GetTemplateHandler(config_obj, "/index.html")
	if err != nil {
		return nil, err
	}
	mux.Handle(api_utils.GetBasePath(config_obj, "/app/index.html"),
		ipFilter(config_obj,
			csrfProtect(config_obj,
				auther.AuthenticateUserHandler(h, acls.READ_RESULTS))))

	// Redirect everything else to the app
	mux.Handle(api_utils.GetBaseDirectory(config_obj),
		api_utils.HandlerFunc(nil,
			func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r,
					api_utils.GetBasePath(config_obj, "/app/index.html"),
					http.StatusTemporaryRedirect)
			}))

	return mux, nil
}

// An api handler which connects to the gRPC service (i.e. it is a
// gRPC client). This is used by the gRPC gateway to relay REST calls
// to the gRPC API. This connection must be identified as the gateway
// identity.
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
					// gRPC metadata can only contain ASCII so we make
					// sure to escape if needed.
					md["USER"] = utils.Quote(username)
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
	gw_cert, err := crypto_utils.ParseX509CertFromPemStr(
		[]byte(config_obj.GUI.GwCertificate))
	if err != nil {
		return nil, err
	}

	_, err = gw_cert.Verify(x509.VerifyOptions{Roots: CA_Pool})
	if err != nil {
		return nil, errors.Wrap(err, 0)
	}

	gw_name := crypto_utils.GetSubjectName(gw_cert)
	if gw_name != utils.GetGatewayName(config_obj) {
		return nil, fmt.Errorf(
			"GUI gRPC proxy Certificate is not correct: %v", gw_name)
	}

	// The API server's TLS address is pinned to the frontend's
	// certificate. We must only connect to the real API server.
	creds := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      CA_Pool,
		ServerName:   utils.GetSuperuserName(config_obj),
	})

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
	}

	// Allow the receive limit to be increased.
	if config_obj.ApiConfig != nil &&
		config_obj.ApiConfig.MaxGrpcRecvSize > 0 {
		opts = append(opts,
			grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(
				int(config_obj.ApiConfig.MaxGrpcRecvSize))))
	}

	bind_addr := grpc_client.GetAPIConnectionString(config_obj)
	err = api_proto.RegisterAPIHandlerFromEndpoint(
		ctx, grpc_proxy_mux, bind_addr, opts)
	if err != nil {
		return nil, err
	}

	reverse_proxy_mux := api_utils.NewServeMux()
	reverse_proxy_mux.Handle(api_utils.GetBasePath(config_obj, "/api/v1/"),
		api_utils.StripPrefix(
			api_utils.GetBasePath(config_obj), grpc_proxy_mux))

	return reverse_proxy_mux, nil
}

func ipFilter(config_obj *config_proto.Config,
	parent http.Handler) http.Handler {
	return authenticators.IpFilter(config_obj, parent)
}
