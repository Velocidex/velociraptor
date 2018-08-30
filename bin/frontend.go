// +build !windows

// Frontend is not built on Windows.
package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"time"

	"github.com/golang/protobuf/proto"
	"gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/velociraptor/api"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/config"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/server"
)

var (
	healthy int32

	// Run the server.
	frontend = app.Command("frontend", "Run the frontend and GUI.")
)

func start_frontend(config_obj *config.Config, server_obj *server.Server) {
	router := http.NewServeMux()
	router.Handle("/healthz", healthz())
	router.Handle("/server.pem", server_pem(config_obj))

	router.Handle("/control", control(server_obj))
	router.Handle("/reader", reader(config_obj, server_obj))

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
		ConnState: func(c net.Conn, cs http.ConnState) {
			key := c.RemoteAddr().String()
			switch cs {
			case http.StateNew, http.StateActive:
				server_obj.SetConn(key, c)
			default:
				server_obj.DelConn(key)
			}
		},
	}

	done := make(chan bool)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)

	go func() {
		<-quit
		server_obj.Info("Server is shutting down...")
		atomic.StoreInt32(&healthy, 0)
		server_obj.NotificationPool.Shutdown()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		server.SetKeepAlivesEnabled(false)
		if err := server.Shutdown(ctx); err != nil {
			kingpin.Fatalf(
				"Could not gracefully shutdown the server: %v\n", err)
		}
		close(done)
	}()

	server_obj.Info("Frontend is ready to handle client requests at %s", listenAddr)
	atomic.StoreInt32(&healthy, 1)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		kingpin.Fatalf("Could not listen on %s: %v\n", listenAddr, err)
	}

	<-done
	server_obj.Info("Server stopped")
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

func server_pem(config_obj *config.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		flusher.Flush()

		w.Write([]byte(config_obj.Frontend.Certificate))
	})
}

func control(server_obj *server.Server) http.Handler {
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
			response, _, err := server_obj.Process(req.Context(), message_info)
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

func reader(config_obj *config.Config, server_obj *server.Server) http.Handler {
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

		deadline := time.After(time.Duration(
			config_obj.Client.MaxPoll) * time.Second)

		// We now write the header and block the client until
		// a notification is sent on the notification pool.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		source := message_info.Source

		// Get a notification for this client from the pool -
		// Must be before the Process() call to prevent race.
		notification := server_obj.NotificationPool.Listen(source)

		// Check for any requests outstanding now.
		response, count, err := server_obj.Process(req.Context(), message_info)
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

		defer server_obj.NotificationPool.Notify(source)
		for {
			select {
			case quit := <-notification:
				if quit {
					logger.Info("reader: quit.")
					return
				}

				logger.Info("reader: notification received.")
				response, _, err := server_obj.Process(
					req.Context(), message_info)
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

				return

			case <-deadline:
				server_obj.NotificationPool.Notify(source)
				logger.Info("reader: Deadline exceeded")

				// Write a pad message every 3 seconds
				// to keep the conenction alive.
			case <-time.After(10 * time.Second):
				n, err := w.Write(serialized_pad)
				if err != nil {
					logger.Info("reader: Error %v", err)
					return
				}

				flusher.Flush()
				logger.Info("reader: heartbeat %d", n)
			}
		}
	})
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		if command == frontend.FullCommand() {
			config_obj, err := get_server_config(*config_path)
			kingpin.FatalIfError(err, "Unable to load config file")

			server_obj, err := server.NewServer(config_obj)
			kingpin.FatalIfError(err, "Unable to create server")

			// Part the artifacts database to detect errors early.
			_, err = artifacts.GetGlobalRepository(config_obj)
			kingpin.FatalIfError(err, "Unable to load artifact database")

			go func() {
				err := api.StartServer(config_obj, server_obj)
				kingpin.FatalIfError(
					err, "Unable to start API server")
			}()
			go func() {
				err := api.StartHTTPProxy(config_obj)
				kingpin.FatalIfError(
					err, "Unable to start HTTP Proxy server")
			}()

			start_frontend(config_obj, server_obj)
			return true
		}
		return false
	})
}
