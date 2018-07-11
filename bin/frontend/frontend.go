//
package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/golang/protobuf/proto"
	"gopkg.in/alecthomas/kingpin.v2"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"time"
	"www.velocidex.com/golang/velociraptor/api"
	"www.velocidex.com/golang/velociraptor/config"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/server"
)

var (
	frontend    = kingpin.Command("frontend", "Run the frontend.")
	config_path = kingpin.Flag("config", "The Configuration file").
			Required().String()

	api_command   = kingpin.Command("api", "Call an API method.")
	api_call_name = api_command.Arg("method", "The method name to call.").
			Required().String()

	healthy int32
)

func validateConfig(configuration *config.Config) error {
	if configuration.Frontend_certificate == nil {
		return errors.New("Configuration does not specify a frontend certificate.")
	}

	return nil
}

func main() {
	switch kingpin.Parse() {
	case "frontend":
		config_obj, err := get_config(*config_path)
		kingpin.FatalIfError(err, "Unable to load config file")
		go func() {
			err := api.StartServer(config_obj)
			kingpin.FatalIfError(err, "Unable to start API server")
		}()
		go func() {
			err := api.StartHTTPProxy(config_obj)
			kingpin.FatalIfError(err, "Unable to start HTTP Proxy server")
		}()

		start_frontend(config_obj)

	case "api":
		config_obj, err := get_config(*config_path)
		kingpin.FatalIfError(err, "Unable to load config file")
		call_api(config_obj, *api_call_name)
	}
}

func get_config(config_path string) (*config.Config, error) {
	config_obj := config.GetDefaultConfig()
	err := config.LoadConfig(config_path, config_obj)
	if err == nil {
		err = validateConfig(config_obj)
	}

	return config_obj, err
}

func start_frontend(config_obj *config.Config) {
	server_obj, err := server.NewServer(config_obj)
	kingpin.FatalIfError(err, "Unable to create server")

	router := http.NewServeMux()
	router.Handle("/healthz", healthz())
	router.Handle("/server.pem", server_pem(config_obj))

	router.Handle("/control", control(server_obj))

	listenAddr := fmt.Sprintf(
		"%s:%d",
		*config_obj.Frontend_bind_address,
		*config_obj.Frontend_bind_port)

	server := &http.Server{
		Addr:        listenAddr,
		Handler:     logging.GetLoggingHandler(config_obj)(router),
		ReadTimeout: 5 * time.Second,
		// Our write timeout is controlled by the handler.
		WriteTimeout: 900 * time.Second,
		IdleTimeout:  15 * time.Second,
	}

	done := make(chan bool)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)

	go func() {
		<-quit
		server_obj.Info("Server is shutting down...")
		atomic.StoreInt32(&healthy, 0)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		server.SetKeepAlivesEnabled(false)
		if err := server.Shutdown(ctx); err != nil {
			kingpin.Fatalf(
				"Could not gracefully shutdown the server: %v\n", err)
		}
		close(done)
	}()

	server_obj.Info("Server is ready to handle requests at %s", listenAddr)
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

		w.Write([]byte(*config_obj.Frontend_certificate))
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
			server_obj.Error("Unable to read body")
			http.Error(w, "", http.StatusServiceUnavailable)
			return
		}

		message_info, err := server_obj.Decrypt(req.Context(), body)
		if err != nil {
			// If we can not decrypt the message because
			// we do not know about this client, we need
			// to indicate to the client to start the
			// enrolment process.
			if err.Error() == "Enrolment" {
				http.Error(
					w,
					"Please Enrol",
					http.StatusNotAcceptable)
				return
			}

			server_obj.Error("Unable to process: %s", err.Error())
			http.Error(w, "", http.StatusServiceUnavailable)
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
			response, err := server_obj.Process(req.Context(), message_info)
			if err != nil {
				server_obj.Error("Error: %s", err.Error())
			} else {
				sync <- response
			}
			close(sync)
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

func call_api(config_obj *config.Config, method string) {

}
