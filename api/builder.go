package api

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/acme/autocert"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/server"
	"www.velocidex.com/golang/velociraptor/services"
)

// Builder builds a new GUI and Frontend server based on configuration
// options. We support several modes:
// 1. Autocert mode uses Let's Encrypt for SSL certificate provisioning.
// 2. Otherwise Self signed SSL is used for both Frontend and GUI.
// 3. If the GUI and Frontend are on the same port, we build a single
// unified server but otherwise we build two separate servers.
//
// If `Frontend.use_plain_http` is set, we bring the frontend up with
// plain HTTP server. This is useful to SSL offload to a reverse proxy
// like nginx. NOTE: you will need to key nginx with Velociraptor's
// self signed certificates or a proper cert - remember to adjust the
// client's `use_self_signed_ssl` parameters as appropriate.
//
// If you are using a reverse proxy in front of Velociraptor make sure
// you disable buffering. With nginx the setting is
// `proxy_request_buffering off`
// http://nginx.org/en/docs/http/ngx_http_proxy_module.html#proxy_request_buffering
type Builder struct {
	config_obj *config_proto.Config

	server_obj *server.Server

	GUIPort, FrontendPort uint32
	AutocertCertCache     string
}

func (self *Builder) StartServer(ctx context.Context, wg *sync.WaitGroup) error {
	// Always start the prometheus monitoring service
	err := StartMonitoringService(ctx, wg, self.config_obj)
	if err != nil {
		return err
	}

	// Start in autocert mode, only put the GUI behind autocert if the GUI port is 443.
	if self.AutocertCertCache != "" && self.config_obj.GUI != nil &&
		self.config_obj.GUI.BindPort == 443 {
		return self.WithAutocertGUI(ctx, wg)
	}

	// Start in autocert mode, but only sign the frontend.
	if self.AutocertCertCache != "" {
		return self.withAutoCertFrontendSelfSignedGUI(ctx, wg, self.config_obj, self.server_obj)
	}

	// All services are sharing the same port.
	if self.GUIPort == self.FrontendPort {
		return startSharedSelfSignedFrontend(ctx, wg, self.config_obj, self.server_obj)
	}

	return startSelfSignedFrontend(ctx, wg, self.config_obj, self.server_obj)
}

func NewServerBuilder(ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) (*Builder, error) {
	result := &Builder{config_obj: config_obj}

	// Create a new server
	server_obj, err := server.NewServer(ctx, config_obj, wg)
	if err != nil {
		return nil, err
	}

	result.server_obj = server_obj

	// Fill in the usual defaults.
	result.AutocertCertCache = config_obj.AutocertCertCache
	result.GUIPort = config_obj.GUI.BindPort
	result.FrontendPort = config_obj.Frontend.BindPort

	return result, nil
}

func (self *Builder) Close() {
	if self.server_obj != nil {
		self.server_obj.Close()
	}
}

func (self *Builder) WithAPIServer(ctx context.Context, wg *sync.WaitGroup) error {
	return startAPIServer(ctx, wg, self.config_obj, self.server_obj)
}

func (self *Builder) withAutoCertFrontendSelfSignedGUI(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config,
	server_obj *server.Server) error {

	if self.config_obj.Frontend == nil || self.config_obj.GUI == nil {
		return errors.New("Frontend not configured")
	}

	logger := logging.GetLogger(config_obj, &logging.GUIComponent)
	logger.Info("Autocert is enabled but GUI port is not 443, starting Frontend with autocert and GUI with self signed.")

	if config_obj.Frontend.ServerServices.GuiServer && config_obj.GUI != nil {
		mux := http.NewServeMux()

		router, err := PrepareGUIMux(ctx, config_obj, mux)
		if err != nil {
			return err
		}

		// Start the GUI separately on a different port.
		if config_obj.GUI.UsePlainHttp {
			err = StartHTTPGUI(ctx, wg, config_obj, router)
		} else {
			err = StartSelfSignedGUI(ctx, wg, config_obj, router)
		}
		if err != nil {
			return err
		}
	}

	// Launch a server for the frontend.
	mux := http.NewServeMux()

	err := server.PrepareFrontendMux(
		config_obj, server_obj, mux)
	if err != nil {
		return err
	}

	return StartFrontendWithAutocert(ctx, wg,
		self.config_obj, self.server_obj, mux)

}

// When the GUI and Frontend share the same port we start them with
// the same server.
func (self *Builder) WithAutocertGUI(
	ctx context.Context,
	wg *sync.WaitGroup) error {

	if self.config_obj.Frontend == nil || self.config_obj.GUI == nil {
		return errors.New("Frontend not configured")
	}

	mux := http.NewServeMux()

	err := server.PrepareFrontendMux(self.config_obj, self.server_obj, mux)
	if err != nil {
		return err
	}

	router, err := PrepareGUIMux(ctx, self.config_obj, mux)
	if err != nil {
		return err
	}

	// Start comms over https.
	return StartFrontendWithAutocert(ctx, wg,
		self.config_obj, self.server_obj, router)
}

// When the GUI and Frontend share the same port we start them with
// the same server.
func startSharedSelfSignedFrontend(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config,
	server_obj *server.Server) error {
	mux := http.NewServeMux()

	if config_obj.Frontend == nil || config_obj.GUI == nil {
		return errors.New("Frontend not configured")
	}

	err := server.PrepareFrontendMux(config_obj, server_obj, mux)
	if err != nil {
		return err
	}
	router, err := PrepareGUIMux(ctx, config_obj, mux)
	if err != nil {
		return err
	}

	// Combine both frontend and GUI on HTTP server.
	if config_obj.GUI.UsePlainHttp && config_obj.Frontend.UsePlainHttp {
		return StartFrontendPlainHttp(
			ctx, wg, config_obj, server_obj, mux)
	}

	return StartFrontendHttps(ctx, wg,
		config_obj, server_obj, router)
}

// Start the Frontend and GUI on different ports using different
// server objects.
func startSelfSignedFrontend(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config,
	server_obj *server.Server) error {

	if config_obj.Frontend == nil {
		return errors.New("Frontend not configured")
	}

	// Launch a new server for the GUI.
	if config_obj.Frontend.ServerServices.GuiServer {
		mux := http.NewServeMux()

		router, err := PrepareGUIMux(ctx, config_obj, mux)
		if err != nil {
			return err
		}

		// Start the GUI separately on a different port.
		if config_obj.GUI.UsePlainHttp {
			err = StartHTTPGUI(ctx, wg, config_obj, router)
		} else {
			err = StartSelfSignedGUI(ctx, wg, config_obj, router)
		}
		if err != nil {
			return err
		}
	}

	// Launch a server for the frontend.
	mux := http.NewServeMux()

	server.PrepareFrontendMux(
		config_obj, server_obj, mux)

	if config_obj.Frontend.UsePlainHttp {
		return StartFrontendPlainHttp(
			ctx, wg, config_obj, server_obj, mux)
	}

	// Start comms over https.
	return StartFrontendHttps(ctx, wg,
		config_obj, server_obj, mux)
}

func getCertificates(config_obj *config_proto.Config) ([]tls.Certificate, error) {
	// If we need to read TLS certs from a file then do it now.
	if config_obj.Frontend.TlsCertificateFilename != "" {
		cert, err := tls.LoadX509KeyPair(
			config_obj.Frontend.TlsCertificateFilename,
			config_obj.Frontend.TlsPrivateKeyFilename)
		if err != nil {
			return nil, err
		}
		return []tls.Certificate{cert}, nil
	}

	cert, err := tls.X509KeyPair(
		[]byte(config_obj.Frontend.Certificate),
		[]byte(config_obj.Frontend.PrivateKey))
	if err != nil {
		return nil, err
	}

	return []tls.Certificate{cert}, nil
}

// Starts the frontend over HTTPS.
func StartFrontendHttps(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config,
	server_obj *server.Server,
	router http.Handler) error {

	if config_obj.Frontend == nil {
		return errors.New("Frontend server not configured")
	}

	certs, err := getCertificates(config_obj)
	if err != nil {
		return err
	}

	listenAddr := fmt.Sprintf(
		"%s:%d",
		config_obj.Frontend.BindAddress,
		config_obj.Frontend.BindPort)

	server := &http.Server{
		Addr:     listenAddr,
		Handler:  router,
		ErrorLog: logging.NewPlainLogger(config_obj, &logging.FrontendComponent),

		// https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/
		ReadTimeout:  500 * time.Second,
		WriteTimeout: 900 * time.Second,
		IdleTimeout:  150 * time.Second,
		TLSConfig: &tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: certs,
			CurvePreferences: []tls.CurveID{tls.CurveP521,
				tls.CurveP384, tls.CurveP256},

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

	wg.Add(1)
	go func() {
		defer wg.Done()
		server_obj.Info("Frontend is ready to handle client TLS requests at <green>https://%s:%d/",
			get_hostname(config_obj.Frontend.Hostname, config_obj.Frontend.BindAddress),
			config_obj.Frontend.BindPort)

		atomic.StoreInt32(&server_obj.Healthy, 1)

		err := server.ListenAndServeTLS("", "")
		if err != nil && err != http.ErrServerClosed {
			server_obj.Fatal("Frontend server error %v", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()

		server_obj.Info("<red>Shutting down</> frontend")
		atomic.StoreInt32(&server_obj.Healthy, 0)

		time_ctx, cancel := context.WithTimeout(
			context.Background(), 10*time.Second)
		defer cancel()

		server.SetKeepAlivesEnabled(false)

		err := server.Shutdown(time_ctx)
		if err != nil {
			server_obj.Error("Frontend server error %v", err)
		}
	}()

	return nil
}

// Starts the frontend over HTTPS.
func StartFrontendPlainHttp(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config,
	server_obj *server.Server,
	router http.Handler) error {
	if config_obj.Frontend == nil {
		return errors.New("Frontend server not configured")
	}

	listenAddr := fmt.Sprintf(
		"%s:%d",
		config_obj.Frontend.BindAddress,
		config_obj.Frontend.BindPort)

	server := &http.Server{
		Addr:     listenAddr,
		Handler:  router,
		ErrorLog: logging.NewPlainLogger(config_obj, &logging.FrontendComponent),

		// https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/
		ReadTimeout:  500 * time.Second,
		WriteTimeout: 900 * time.Second,
		IdleTimeout:  300 * time.Second,
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		server_obj.Info("Frontend is ready to handle requests at <green>http://%s:%d/",
			get_hostname(config_obj.Frontend.Hostname, config_obj.Frontend.BindAddress),
			config_obj.Frontend.BindPort)

		atomic.StoreInt32(&server_obj.Healthy, 1)

		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			server_obj.Fatal("Frontend server error %v", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()

		server_obj.Info("<red>Shutting down</> frontend")
		atomic.StoreInt32(&server_obj.Healthy, 0)

		time_ctx, cancel := context.WithTimeout(
			context.Background(), 10*time.Second)
		defer cancel()

		server.SetKeepAlivesEnabled(false)
		notifier := services.GetNotifier()
		if notifier != nil {
			err := notifier.NotifyAllListeners(config_obj)
			if err != nil {
				server_obj.Error("Frontend server error %v", err)
			}
		}
		err := server.Shutdown(time_ctx)
		if err != nil {
			server_obj.Error("Frontend server error %v", err)
		}
	}()

	return nil
}

// Starts both Frontend and GUI on the same server. This is used in
// Autocert configuration.
func StartFrontendWithAutocert(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config,
	server_obj *server.Server,
	mux http.Handler) error {

	if config_obj.Frontend == nil {
		return errors.New("Frontend server not configured")
	}

	logger := logging.Manager.GetLogger(config_obj, &logging.GUIComponent)

	cache_dir := config_obj.AutocertCertCache
	certManager := autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(config_obj.Frontend.Hostname),
		Cache:      autocert.DirCache(cache_dir),
	}

	server := &http.Server{
		// ACME protocol requires TLS be served over port 443.
		Addr:     ":https",
		Handler:  mux,
		ErrorLog: logging.NewPlainLogger(config_obj, &logging.FrontendComponent),

		// https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/
		ReadTimeout:  500 * time.Second,
		WriteTimeout: 900 * time.Second,
		IdleTimeout:  300 * time.Second,
		TLSConfig: &tls.Config{
			MinVersion:     tls.VersionTLS12,
			GetCertificate: certManager.GetCertificate,
		},
	}

	// We must have port 80 open to serve the HTTP 01 challenge.
	go func() {
		err := http.ListenAndServe(":http", certManager.HTTPHandler(nil))
		if err != nil {
			logger := logging.Manager.GetLogger(config_obj, &logging.GUIComponent)
			logger.Error("Failed to bind to http server: %v", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		server_obj.Info("Frontend is ready to handle client requests at <green>https://%s/",
			get_hostname(config_obj.Frontend.Hostname, config_obj.Frontend.BindAddress))
		atomic.StoreInt32(&server_obj.Healthy, 1)

		err := server.ListenAndServeTLS("", "")
		if err != nil && err != http.ErrServerClosed {
			server_obj.Fatal("Frontend server error", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()

		server_obj.Info("<red>Stopping Frontend Server")
		atomic.StoreInt32(&server_obj.Healthy, 0)

		timeout_ctx, cancel := context.WithTimeout(
			context.Background(), 10*time.Second)
		defer cancel()

		server.SetKeepAlivesEnabled(false)
		notifier := services.GetNotifier()
		if notifier != nil {
			_ = notifier.NotifyAllListeners(config_obj)
		}
		err := server.Shutdown(timeout_ctx)
		if err != nil {
			logger.Error("Frontend shutdown error: %v", err)
		}
		server_obj.Info("Shutdown frontend")
	}()

	return nil
}

func StartHTTPGUI(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config, mux http.Handler) error {

	if config_obj.GUI == nil {
		return errors.New("GUI server not configured")
	}

	logger := logging.Manager.GetLogger(config_obj, &logging.GUIComponent)

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
	}

	logger.Info("GUI is ready to handle HTTP requests on <green>http://%s:%d/",
		get_hostname(config_obj.Frontend.Hostname, config_obj.GUI.BindAddress),
		config_obj.GUI.BindPort)

	wg.Add(1)
	go func() {
		defer wg.Done()

		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			logger.Error("GUI Server error: %v", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()

		logger.Info("<red>Stopping GUI Server")
		timeout_ctx, cancel := context.WithTimeout(
			context.Background(), 10*time.Second)
		defer cancel()

		server.SetKeepAlivesEnabled(false)
		err := server.Shutdown(timeout_ctx)
		if err != nil {
			logger.Error("GUI shutdown error: %v", err)
		}
	}()

	return nil
}

func StartSelfSignedGUI(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config, mux http.Handler) error {
	logger := logging.Manager.GetLogger(config_obj, &logging.GUIComponent)
	if config_obj.GUI == nil {
		return errors.New("GUI server not configured")
	}

	certs, err := getCertificates(config_obj)
	if err != nil {
		return err
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
			MinVersion: tls.VersionTLS12,
			CurvePreferences: []tls.CurveID{tls.CurveP521,
				tls.CurveP384, tls.CurveP256},
			Certificates:             certs,
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

	logger.Info("GUI is ready to handle TLS requests on <green>https://%s:%d/",
		get_hostname(config_obj.Frontend.Hostname, config_obj.GUI.BindAddress),
		config_obj.GUI.BindPort)

	wg.Add(1)
	go func() {
		defer wg.Done()

		err := server.ListenAndServeTLS("", "")
		if err != nil && err != http.ErrServerClosed {
			logger.Error("GUI Server error: %v", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()

		logger.Info("<red>Stopping GUI Server")
		timeout_ctx, cancel := context.WithTimeout(
			context.Background(), 10*time.Second)
		defer cancel()

		server.SetKeepAlivesEnabled(false)
		err := server.Shutdown(timeout_ctx)
		if err != nil {
			logger.Error("GUI shutdown error: %v", err)
		}
	}()

	return nil
}

func get_hostname(fe_hostname, bind_addr string) string {
	if bind_addr == "0.0.0.0" || bind_addr == "" || bind_addr == "::" {
		return fe_hostname
	}
	return bind_addr
}
