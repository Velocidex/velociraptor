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
package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"time"

	"www.velocidex.com/golang/velociraptor/utils"

	"github.com/golang/protobuf/proto"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golang.org/x/crypto/acme/autocert"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
)

var (
	healthy            int32
	currentConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "client_comms_current_connections",
		Help: "Number of currently connected clients.",
	})
)

func PrepareFrontendMux(
	config_obj *config_proto.Config,
	server_obj *Server,
	router *http.ServeMux) {
	router.Handle("/healthz", healthz())
	router.Handle("/server.pem", server_pem(config_obj))
	router.Handle("/control", control(server_obj))
	router.Handle("/reader", reader(config_obj, server_obj))

	if config_obj.Frontend.PublicPath != "" {
		router.Handle(
			"/public/", http.StripPrefix("/public/",
				http.FileServer(http.Dir(
					config_obj.Frontend.PublicPath,
				))))
	}
}

// Starts the frontend over HTTP. Velociraptor uses its own encryption
// protocol so using HTTP is quite safe.
func StartFrontendHttp(
	config_obj *config_proto.Config,
	server_obj *Server,
	router *http.ServeMux) error {
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
		IdleTimeout:  15 * time.Second,
	}

	wg := &sync.WaitGroup{}
	InstallSignalHandler(config_obj, server_obj, server, wg)
	server_obj.Info("Frontend is ready to handle client requests at %s", listenAddr)

	err := server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}

	wg.Wait()
	server_obj.Info("Server stopped")

	return nil
}

// Starts the frontend over HTTPS.
func StartFrontendHttps(
	config_obj *config_proto.Config,
	server_obj *Server,
	router *http.ServeMux) error {

	cert, err := tls.X509KeyPair(
		[]byte(config_obj.Frontend.Certificate),
		[]byte(config_obj.Frontend.PrivateKey))
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

	wg := &sync.WaitGroup{}
	InstallSignalHandler(config_obj, server_obj, server, wg)
	server_obj.Info("Frontend is ready to handle client TLS requests at %s", listenAddr)

	err = server.ListenAndServeTLS("", "")
	if err != nil && err != http.ErrServerClosed {
		return err
	}

	wg.Wait()
	server_obj.Info("Server stopped")

	return nil
}

// Install a signal handler which will shutdown the server gracefully.
func InstallSignalHandler(
	config_obj *config_proto.Config,
	server_obj *Server,
	server *http.Server,
	wg *sync.WaitGroup) {

	// Wait for signal. When signal is received we shut down the
	// server.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	wg.Add(1)

	go func() {
		defer wg.Done()

		// Start all the services and shut them down when we
		// are done.
		logger := logging.Manager.GetLogger(
			config_obj, &logging.GUIComponent)

		// When we exit from here, unwind the server.
		defer func() {
			atomic.StoreInt32(&healthy, 0)

			// Server must shutdown in a reasonable time.
			ctx, cancel := context.WithTimeout(
				context.Background(), 10*time.Second)
			defer cancel()

			server.SetKeepAlivesEnabled(false)
			logger.Info("Server is shutting down...")

			// Notify all the currently connected clients we need
			// to shut down.
			server_obj.NotificationPool.NotifyAll()
			err := server.Shutdown(ctx)
			if err != nil {
				logger.Error(
					"Could not gracefully shutdown the server: ",
					err)
			}
		}()

		manager, err := services.StartServices(
			config_obj,
			server_obj.NotificationPool)
		if err != nil {
			logger.Error("Failed starting services: ", err)
			return
		}
		defer manager.Close()

		// Wait for the signal on this channel then return.
		<-quit
	}()

	atomic.StoreInt32(&healthy, 1)
}

func StartTLSServer(
	config_obj *config_proto.Config,
	server_obj *Server,
	mux *http.ServeMux) error {
	logger := logging.Manager.GetLogger(config_obj, &logging.GUIComponent)

	if config_obj.GUI.BindPort != 443 {
		logger.Info("Autocert specified - will listen on ports 443 and 80. "+
			"I will ignore specified GUI port at %v",
			config_obj.GUI.BindPort)
	}

	if config_obj.Frontend.BindPort != 443 {
		logger.Info("Autocert specified - will listen on ports 443 and 80. "+
			"I will ignore specified Frontend port at %v",
			config_obj.GUI.BindPort)
	}

	cache_dir := config_obj.AutocertCertCache
	if cache_dir == "" {
		cache_dir = "/tmp/velociraptor_cache"
	}

	certManager := autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(config_obj.AutocertDomain),
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
		IdleTimeout:  15 * time.Second,
		TLSConfig: &tls.Config{
			MinVersion:     tls.VersionTLS12,
			GetCertificate: certManager.GetCertificate,
		},
	}

	// We must have port 80 open to serve the HTTP 01 challenge.
	go http.ListenAndServe(":http", certManager.HTTPHandler(nil))

	wg := &sync.WaitGroup{}
	InstallSignalHandler(config_obj, server_obj, server, wg)

	server_obj.Info("Frontend is ready to handle client requests using HTTPS")
	err := server.ListenAndServeTLS("", "")
	if err != nil && err != http.ErrServerClosed {
		return err
	}

	wg.Wait()
	server_obj.Info("Server stopped")
	return nil
}

func healthz() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadInt32(&healthy) == 1 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
}

func server_pem(config_obj *config_proto.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		flusher.Flush()

		w.Write([]byte(config_obj.Frontend.Certificate))
	})
}

// This handler is used to receive messages from the client to the
// server. These connections are short lived - the client will just
// post its message and then disconnect.
func control(server_obj *Server) http.Handler {
	pad := &crypto_proto.ClientCommunication{}
	pad.Padding = append(pad.Padding, 0)
	serialized_pad, _ := proto.Marshal(pad)
	logger := logging.GetLogger(server_obj.config, &logging.FrontendComponent)

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			panic("http handler is not a flusher")
		}

		server_obj.StartConcurrencyControl()
		defer server_obj.EndConcurrencyControl()

		body, err := ioutil.ReadAll(
			io.LimitReader(req.Body, int64(server_obj.config.
				Frontend.MaxUploadSize*2)))
		if err != nil {
			logger.Debug("Unable to read body from %v: %+v (read %v)",
				req.RemoteAddr, err, len(body))
			http.Error(w, "", http.StatusServiceUnavailable)
			return
		}

		message_info, err := server_obj.Decrypt(req.Context(), body)
		if err != nil {
			logger.Debug("Unable to decrypt body from %v: %+v "+
				"(%v out of max %v)",
				req.RemoteAddr, err, len(body), server_obj.config.
					Frontend.MaxUploadSize*2)
			// Just plain reject with a 403.
			http.Error(w, "", http.StatusForbidden)
			return
		}
		message_info.RemoteAddr = utils.RemoteAddr(req, server_obj.config.Frontend.GetProxyHeader())
		logger.Debug("Received a post of length %v from %v (%v)", len(body),
			message_info.RemoteAddr, message_info.Source)

		// Very few Unauthenticated client messages are valid
		// - currently only enrolment requests.
		if !message_info.Authenticated {
			err := server_obj.ProcessUnauthenticatedMessages(
				req.Context(), message_info)
			if err == nil {
				// We need to indicate to the client
				// to start the enrolment
				// process. Since the client can not
				// read anything from us (because we
				// can not encrypt for it), we
				// indicate this by providing it with
				// an HTTP error code.
				http.Error(
					w,
					"Please Enrol",
					http.StatusNotAcceptable)
			} else {
				server_obj.Error("Unable to process", err)
				http.Error(w, "", http.StatusServiceUnavailable)
			}
			return
		}

		// From here below we have received the client payload
		// and it should not resend it to us. We need to keep
		// the client blocked until we finish processing the
		// flow as a method of rate limiting the clients. We
		// do this by streaming pad packets to the client,
		// while the flow is processed.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		sync := make(chan []byte)
		go func() {
			defer close(sync)
			response, _, err := server_obj.Process(
				req.Context(), message_info,
				false, // drain_requests_for_client
			)
			if err != nil {
				server_obj.Error("Error:", err)
			} else {
				sync <- response
			}
		}()

		// Spin here on the response being ready, every few
		// seconds we send a pad packet to the client to keep
		// the connection active. The pad packet is sent using
		// chunked encoding and will be assembled by the
		// client into the complete protobuf when it is
		// read. Since protobuf serialization does not have
		// headers, it is safe to just concat multiple binary
		// encodings into a single proto. This means the
		// decoded protobuf will actually contain the full
		// response as well as the padding fields (which are
		// ignored).
		for {
			select {
			case response := <-sync:
				w.Write(response)
				return

			case <-time.After(3 * time.Second):
				w.Write(serialized_pad)
				flusher.Flush()
			}
		}
	})
}

// This handler is used to send messages to the client. This
// connection will persist up to Client.MaxPoll so we always have a
// channel to the client. This allows us to send the client jobs
// immediately with low latency.
func reader(config_obj *config_proto.Config, server_obj *Server) http.Handler {
	pad := &crypto_proto.ClientCommunication{}
	pad.Padding = append(pad.Padding, 0)
	serialized_pad, _ := proto.Marshal(pad)
	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			panic("http handler is not a flusher")
		}

		// Keep track of currently connected clients.
		currentConnections.Inc()
		defer currentConnections.Dec()

		body, err := ioutil.ReadAll(
			io.LimitReader(req.Body, constants.MAX_MEMORY))
		if err != nil {
			server_obj.Error("Unable to read body", err)
			http.Error(w, "", http.StatusServiceUnavailable)
			return
		}

		message_info, err := server_obj.Decrypt(req.Context(), body)
		if err != nil {
			// Just plain reject with a 403.
			http.Error(w, "", http.StatusForbidden)
			return
		}
		message_info.RemoteAddr = req.RemoteAddr

		// Reject unauthenticated messages. This ensures
		// untrusted clients are not allowed to keep
		// connections open.
		if !message_info.Authenticated {
			http.Error(w, "Please Enrol", http.StatusNotAcceptable)
			return
		}

		// Get a notification for this client from the pool -
		// Must be before the Process() call to prevent race.
		source := message_info.Source
		notification, err := server_obj.NotificationPool.Listen(source)
		if err != nil {
			http.Error(w, "Another Client connection exists. "+
				"Only a single instance of the client is "+
				"allowed to connect at the same time.",
				http.StatusConflict)
			return
		}

		// Deadlines are designed to ensure that connections
		// are not blocked for too long (maybe several
		// minutes). This helps to expire connections when the
		// client drops off completely or is behind a proxy
		// which caches the heartbeats. After the deadline we
		// close the connection and expect the client to
		// reconnect again. We add a bit of jitter to ensure
		// clients do not get synchronized.
		wait := time.Duration(config_obj.Client.MaxPoll+
			uint64(rand.Intn(30))) * time.Second

		deadline := time.After(wait)

		// We now write the header and block the client until
		// a notification is sent on the notification pool.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		// Remove the notification from the pool when we exit
		// here.
		defer server_obj.NotificationPool.Notify(source)

		// Check for any requests outstanding now.
		response, count, err := server_obj.Process(
			req.Context(), message_info,
			true, // drain_requests_for_client
		)
		if err != nil {
			server_obj.Error("Error:", err)
			return
		}
		if count > 0 {
			// Send the new messages to the client
			// and finish the request off.
			n, err := w.Write(response)
			if err != nil || n < len(serialized_pad) {
				server_obj.Info("reader: Error %v", err)
			}
			return
		}

		// Figure out when the client drops the connection so
		// we can exit.
		close_notify := w.(http.CloseNotifier).CloseNotify()

		for {
			select {
			case <-close_notify:
				return

			case quit := <-notification:
				if quit {
					logger.Info("reader: quit.")
					return
				}
				response, _, err := server_obj.Process(
					req.Context(),
					message_info,
					true, // drain_requests_for_client
				)
				if err != nil {
					server_obj.Error("Error:", err)
					return
				}

				// Send the new messages to the client
				// and finish the request off.
				n, err := w.Write(response)
				if err != nil || n < len(serialized_pad) {
					logger.Debug("reader: Error %v", err)
				}

				flusher.Flush()
				return

			case <-deadline:
				// Notify ourselves, this will trigger
				// an empty response to be written and
				// the connection to be terminated
				// (case above).
				server_obj.NotificationPool.Notify(source)
				logger.Info("reader: Deadline exceeded")

				// Write a pad message every 3 seconds
				// to keep the conenction alive.
			case <-time.After(10 * time.Second):
				_, err := w.Write(serialized_pad)
				if err != nil {
					logger.Info("reader: Error %v", err)
					return
				}

				flusher.Flush()
			}
		}
	})
}
