package main

import (
	"log"
	"net/http"
	_ "net/http/pprof"

	logging "www.velocidex.com/golang/velociraptor/logging"
)

var (
	debug_flag = app.Flag("debug", "Enables debug and profile server.").Bool()
)

func doDebug() {
	config_obj := get_config_or_default()
	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
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
