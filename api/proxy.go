package api

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/logging"
)

// Prepares a mux by adding handler required for the GUI.
func PrepareMux(config_obj *api_proto.Config, mux *http.ServeMux) error {
	ctx := context.Background()
	h, err := GetAPIHandler(ctx, config_obj)
	if err != nil {
		return err
	}

	mux.Handle("/api/", checkUserCredentialsHandler(config_obj, h))
	mux.Handle("/api/v1/download/", checkUserCredentialsHandler(
		config_obj, flowResultDownloadHandler(config_obj)))
	mux.Handle("/api/v1/DownloadHuntResults", checkUserCredentialsHandler(
		config_obj, huntResultDownloadHandler(config_obj)))
	mux.Handle("/api/v1/DownloadVFSFile/", checkUserCredentialsHandler(
		config_obj, vfsFileDownloadHandler(config_obj)))
	mux.Handle("/api/v1/DownloadVFSFolder", checkUserCredentialsHandler(
		config_obj, vfsFolderDownloadHandler(config_obj)))

	// Assets etc do not need auth.
	install_static_assets(config_obj, mux)

	h, err = GetTemplateHandler(config_obj, "/static/templates/app.html")
	if err != nil {
		return err
	}
	mux.Handle("/app.html", checkUserCredentialsHandler(config_obj, h))

	h, err = GetTemplateHandler(config_obj, "/static/templates/index.html")
	if err != nil {
		return err
	}

	// No Auth on / which is a redirect to app.html anyway.
	mux.Handle("/", h)

	return MaybeAddOAuthHandlers(config_obj, mux)
}

// Starts a HTTP Server (non encrypted) using the passed in mux. It is
// not recommended to export the HTTP port to an external interface
// since it is not encrypted. If you want to use HTTP you should
// listen on localhost and port forward over ssh.
func StartHTTPProxy(config_obj *api_proto.Config, mux *http.ServeMux) error {
	logger := logging.Manager.GetLogger(config_obj, &logging.GUIComponent)
	if config_obj.GUI.BindAddress != "127.0.0.1" {
		logger.Info("GUI is not encrypted and listening on public interfact. " +
			"This is not secure. Please enable TLS.")
	}

	listenAddr := fmt.Sprintf("%s:%d",
		config_obj.GUI.BindAddress,
		config_obj.GUI.BindPort)

	logger.WithFields(
		logrus.Fields{
			"listenAddr": listenAddr,
		}).Info("GUI is ready to handle requests")

	return http.ListenAndServe(listenAddr, mux)
}

func StartSelfSignedHTTPSProxy(config_obj *api_proto.Config, mux *http.ServeMux) error {
	logger := logging.Manager.GetLogger(config_obj, &logging.GUIComponent)

	cert, err := tls.X509KeyPair(
		[]byte(config_obj.Frontend.Certificate),
		[]byte(config_obj.Frontend.PrivateKey))
	if err != nil {
		return err
	}

	listenAddr := fmt.Sprintf("%s:%d",
		config_obj.GUI.BindAddress,
		config_obj.GUI.BindPort)

	server := &http.Server{
		Addr:    listenAddr,
		Handler: mux,

		// https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/
		ReadTimeout:  5 * time.Second,
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

	return server.ListenAndServeTLS("", "")
}

type _templateArgs struct {
	Timestamp  int64
	Heading    string
	Help_url   string
	Report_url string
	Version    string
}

// An api handler which connects to the gRPC service (i.e. it is a
// gRPC client).
func GetAPIHandler(
	ctx context.Context,
	config_obj *api_proto.Config) (http.Handler, error) {

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

	// We use the Frontend's certificate because this connection
	// represents an internal connection.
	cert, err := tls.X509KeyPair(
		[]byte(config_obj.Frontend.Certificate),
		[]byte(config_obj.Frontend.PrivateKey))
	if err != nil {
		return nil, err
	}

	// Authenticate API clients using certificates.
	CA_Pool := x509.NewCertPool()
	CA_Pool.AppendCertsFromPEM([]byte(config_obj.Client.CaCertificate))

	creds := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      CA_Pool,
		ServerName:   constants.FRONTEND_NAME,
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
