package main

import (
	"gopkg.in/alecthomas/kingpin.v2"
	"log"
	"net/http"
	_ "net/http/pprof"
	config "www.velocidex.com/golang/velociraptor/config"
	logging "www.velocidex.com/golang/velociraptor/logging"
)

var (
	debug_flag = app.Flag("debug", "Enables debug and profile server.").Bool()
)

func doDebug() {
	config_obj, err := config.LoadClientConfig(*config_path)
	kingpin.FatalIfError(err, "Unable to load config file")

	logger := logging.NewLogger(config_obj)
	logger.Info("Starting debug server on port 6060")

	go func() {
		log.Println(http.ListenAndServe("0.0.0.0:6060", nil))
	}()
}

func init() {
	// Add this first to ensure it always runs.
	command_handlers = append([]CommandHandler{
		func(command string) bool {
			if *debug_flag {
				doDebug()
			}
			return false
		},
	}, command_handlers...)
}
