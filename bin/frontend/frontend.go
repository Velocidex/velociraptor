//
package main

import (
	"context"
	"errors"
	"fmt"
	"gopkg.in/alecthomas/kingpin.v2"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"time"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/flows"
	flow_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/server"
	//	utils "www.velocidex.com/golang/velociraptor/testing"
)

var (
	frontend    = kingpin.Command("frontend", "Run the frontend.")
	config_path = kingpin.Flag("config", "The Configuration file").
			Required().String()

	flow           = kingpin.Command("flow", "Start a flow on a client.")
	flow_client_id = flow.Arg("client_id", "The client ID to launch.").
			Required().String()
	flow_name = flow.Arg("flow_name", "The name of the flow to launch.").
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
		start_frontend(*config_path)
	case "flow":
		start_flow(*config_path, *flow_client_id, *flow_name)
	}
}

func start_flow(config_path string, client_id string, flow_name string) {
	config_obj := config.GetDefaultConfig()
	err := config.LoadConfig(config_path, config_obj)
	if err == nil {
		err = validateConfig(config_obj)
	}

	flow_runner_args := &flow_proto.FlowRunnerArgs{
		ClientId: client_id,
		FlowName: flow_name,
	}

	flow_id, err := flows.StartFlow(config_obj, flow_runner_args)
	kingpin.FatalIfError(err, "Unable to start flow")

	logger := logging.NewLogger(config_obj)
	logger.Info("Launched flow %s", *flow_id)
}

func start_frontend(config_path string) {
	config_obj := config.GetDefaultConfig()
	err := config.LoadConfig(config_path, config_obj)
	if err == nil {
		err = validateConfig(config_obj)
	}

	kingpin.FatalIfError(err, "Unable to load config file")
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
		Addr:         listenAddr,
		Handler:      logging_handler(server_obj)(router),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
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

		fmt.Fprintln(w, *config_obj.Frontend_certificate)
	})
}

func control(server_obj *server.Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := ioutil.ReadAll(req.Body)
		if err != nil {
			server_obj.Error("Unable to read body")
			http.Error(w, "", http.StatusServiceUnavailable)
			return
		}

		response, err := server_obj.Process(req.Context(), body)
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

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)

		w.Write(response)
	})
}

func logging_handler(server_obj *server.Server) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				server_obj.Info(
					"%s %s %s %s",
					r.Method,
					r.URL.Path,
					r.RemoteAddr,
					r.UserAgent())
			}()
			next.ServeHTTP(w, r)
		})
	}
}
