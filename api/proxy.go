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
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/crypto"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/logging"
)

func AddProxyMux(config_obj *config_proto.Config, mux *http.ServeMux) error {
	logger := logging.Manager.GetLogger(config_obj, &logging.GUIComponent)

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
			handler = checkUserCredentialsHandler(config_obj, handler)
		}

		mux.Handle(reverse_proxy_config.Route, handler)
	}

	return nil
}

// Prepares a mux by adding handler required for the GUI.
func PrepareMux(config_obj *config_proto.Config, mux *http.ServeMux) (http.Handler, error) {
	ctx := context.Background()
	h, err := GetAPIHandler(ctx, config_obj)
	if err != nil {
		return nil, err
	}

	if err = MaybeAddSAMLHandlers(config_obj, mux); err != nil {
		return nil, err
	}

	mux.Handle("/api/", csrfProtect(config_obj,
		checkUserCredentialsHandler(config_obj, h)))

	mux.Handle("/api/v1/DownloadVFSFile", csrfProtect(config_obj,
		checkUserCredentialsHandler(
			config_obj, vfsFileDownloadHandler(config_obj))))

	mux.Handle("/api/v1/DownloadVFSFolder", csrfProtect(config_obj,
		checkUserCredentialsHandler(
			config_obj, vfsFolderDownloadHandler(config_obj))))

	// Serve prepared zip files.
	mux.Handle("/downloads/", csrfProtect(config_obj,
		checkUserCredentialsHandler(
			config_obj, http.FileServer(http.Dir(
				config_obj.Datastore.FilestoreDirectory,
			)))))

	// Serve notebook items
	mux.Handle("/notebooks/", csrfProtect(config_obj,
		checkUserCredentialsHandler(
			config_obj, http.FileServer(http.Dir(
				config_obj.Datastore.FilestoreDirectory,
			)))))

	// A logoff handler forces a logoff for basic auth.
	mux.Handle("/logoff", logoff())

	// Assets etc do not need auth.
	install_static_assets(config_obj, mux)

	// Add reverse proxy support.
	err = AddProxyMux(config_obj, mux)
	if err != nil {
		return nil, err
	}

	h, err = GetTemplateHandler(config_obj, "/static/templates/app.html")
	if err != nil {
		return nil, err
	}
	mux.Handle("/app.html", csrfProtect(config_obj,
		checkUserCredentialsHandler(config_obj, h)))

	h, err = GetTemplateHandler(config_obj, "/static/templates/index.html")
	if err != nil {
		return nil, err
	}

	// No Auth on / which is a redirect to app.html anyway.
	mux.Handle("/", h)

	err = MaybeAddOAuthHandlers(config_obj, mux)
	return mux, err
}

func StartSelfSignedHTTPSProxy(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config, mux http.Handler) {
	logger := logging.Manager.GetLogger(config_obj, &logging.GUIComponent)

	cert, err := tls.X509KeyPair(
		[]byte(config_obj.Frontend.Certificate),
		[]byte(config_obj.Frontend.PrivateKey))
	if err != nil {
		logger.Error("GUI Error", err)
		return
	}

	listenAddr := fmt.Sprintf("%s:%d",
		config_obj.GUI.BindAddress,
		config_obj.GUI.BindPort)

	server := &http.Server{
		Addr:     listenAddr,
		Handler:  mux,
		ErrorLog: logging.NewPlainLogger(config_obj, &logging.FrontendComponent),

		// https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/
		ReadTimeout:  500 * time.Second,
		WriteTimeout: 900 * time.Second,
		IdleTimeout:  15 * time.Second,
		TLSConfig: &tls.Config{
			MinVersion:               tls.VersionTLS12,
			CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
			Certificates:             []tls.Certificate{cert},
			PreferServerCipherSuites: true,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
				tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_RSA_WITH_AES_256_CBC_SHA,
				tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
			},
		},
	}

	logger.WithFields(
		logrus.Fields{
			"listenAddr": listenAddr,
		}).Info("GUI is ready to handle TLS requests")

	wg.Add(1)
	go func() {
		defer wg.Done()

		err := server.ListenAndServeTLS("", "")
		if err != nil && err != http.ErrServerClosed {
			logger.Error("GUI Server error", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()

		logger.Info("Stopping GUI Server")
		timeout_ctx, cancel := context.WithTimeout(
			context.Background(), 10*time.Second)
		defer cancel()

		server.SetKeepAlivesEnabled(false)
		err := server.Shutdown(timeout_ctx)
		if err != nil {
			logger.Error("GUI shutdown error ", err)
		}
		logger.Info("Shutdown GUI")
	}()
}

type _templateArgs struct {
	Timestamp  int64
	Heading    string
	Help_url   string
	Report_url string
	Version    string
	CsrfToken  string
}

// An api handler which connects to the gRPC service (i.e. it is a
// gRPC client).
func GetAPIHandler(
	ctx context.Context,
	config_obj *config_proto.Config) (http.Handler, error) {

	// We need to tell when someone uses HEAD method on our grpc
	// proxy so we need to pass this information from the request
	// to the gRPC server using the gRPC metadata.
	grpc_proxy_mux := runtime.NewServeMux(
		runtime.WithMetadata(
			func(ctx context.Context, req *http.Request) metadata.MD {
				md := map[string]string{
					"METHOD": req.Method,
				}
				username, ok := req.Context().Value(
					contextKeyUser).(string)
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

	if gw_cert.Subject.CommonName != config_obj.API.PinnedGwName {
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

	reverse_proxy_mux := http.NewServeMux()
	reverse_proxy_mux.Handle("/api/v1/", grpc_proxy_mux)

	return reverse_proxy_mux, nil
}
