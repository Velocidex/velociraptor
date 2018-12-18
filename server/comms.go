package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/crypto/acme/autocert"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/flows"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
)

var (
	healthy int32
)

func PrepareFrontendMux(
	config_obj *api_proto.Config,
	server_obj *Server,
	router *http.ServeMux) {
	router.Handle("/healthz", healthz())
	router.Handle("/server.pem", server_pem(config_obj))
	router.Handle("/control", control(server_obj))
	router.Handle("/reader", reader(config_obj, server_obj))
}

// Starts the frontend over HTTP. Velociraptor uses its own encryption
// protocol so using HTTP is quite safe.
func StartFrontendHttp(
	config_obj *api_proto.Config,
	server_obj *Server,
	router *http.ServeMux) error {
	listenAddr := fmt.Sprintf(
		"%s:%d",
		config_obj.Frontend.BindAddress,
		config_obj.Frontend.BindPort)

	server := &http.Server{
		Addr:    listenAddr,
		Handler: logging.GetLoggingHandler(config_obj)(router),

		// https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/
		ReadTimeout:  5 * time.Second,
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

// Install a signal handler which will shutdown the server gracefully.
func InstallSignalHandler(
	config_obj *api_proto.Config,
	server_obj *Server,
	server *http.Server,
	wg *sync.WaitGroup) {

	// Wait for signal. When signal is received we shut down the
	// server.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	go func() {
		wg.Add(1)
		defer wg.Done()

		// Start the hunt dispatcher.
		_, err := flows.GetHuntDispatcher(config_obj)
		if err != nil {
			return
		}

		manager, err := services.StartHuntManager(config_obj)
		if err != nil {
			return
		}
		defer manager.Close()

		// Wait for the signal on this channel.
		<-quit

		logger := logging.NewLogger(config_obj)
		atomic.StoreInt32(&healthy, 0)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		server.SetKeepAlivesEnabled(false)
		logger.Info("Server is shutting down...")

		// Notify all the currently connected clients we need
		// to shut down.
		server_obj.NotificationPool.NotifyAll()
		err = server.Shutdown(ctx)
		if err != nil {
			logger.Error("Could not gracefully shutdown the server: ", err)
		}
	}()

	atomic.StoreInt32(&healthy, 1)
}

func StartTLSServer(
	config_obj *api_proto.Config,
	server_obj *Server,
	mux *http.ServeMux) error {
	logger := logging.NewLogger(config_obj)

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
		Addr:    ":https",
		Handler: logging.GetLoggingHandler(config_obj)(mux),

		// https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 900 * time.Second,
		IdleTimeout:  15 * time.Second,
		TLSConfig: &tls.Config{
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

func server_pem(config_obj *api_proto.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		flusher.Flush()

		w.Write([]byte(config_obj.Frontend.Certificate))
	})
}

func control(server_obj *Server) http.Handler {
	pad := &crypto_proto.ClientCommunication{}
	pad.Padding = append(pad.Padding, 0)
	serialized_pad, _ := proto.Marshal(pad)

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			panic("http handler is not a flusher")
		}

		body, err := ioutil.ReadAll(req.Body)
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

func reader(config_obj *api_proto.Config, server_obj *Server) http.Handler {
	pad := &crypto_proto.ClientCommunication{}
	pad.Padding = append(pad.Padding, 0)
	serialized_pad, _ := proto.Marshal(pad)
	logger := logging.NewLogger(config_obj)

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			panic("http handler is not a flusher")
		}

		body, err := ioutil.ReadAll(req.Body)
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
		// reconnect again.
		deadline := time.After(time.Duration(
			config_obj.Client.MaxPoll) * time.Second)

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

				logger.Info("reader: notification received.")
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
					logger.Info("reader: Error %v", err)
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
