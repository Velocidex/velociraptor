package main

import (
	"fmt"
	"strings"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/search"
	"www.velocidex.com/golang/velociraptor/services"
)

var (
	index_command = app.Command(
		"index", "Manage client index.")

	index_command_rebuild = index_command.Command(
		"rebuild", "Rebuild client index")
)

func doRebuildIndex() {
	config_obj, err := makeDefaultConfigLoader().
		WithRequiredFrontend().LoadAndValidate()
	kingpin.FatalIfError(err, "Unable to load config file")

	sm, err := startEssentialServices(config_obj)
	kingpin.FatalIfError(err, "Starting services.")
	defer sm.Close()

	client_info_manager := services.GetClientInfoManager()
	labeler := services.GetLabeler()

	db, err := datastore.GetDB(config_obj)
	kingpin.FatalIfError(err, "Starting services.")

	// This will be very slow on EFS or large directories but it is
	// necessary.
	clients, err := db.ListChildren(
		config_obj, paths.NewClientPathManager("X").Path().Dir())
	kingpin.FatalIfError(err, "Enumerating clients.")

	for _, client_urn := range clients {
		if client_urn.IsDir() {
			continue
		}

		client_id := client_urn.Base()
		if !strings.HasPrefix(client_id, "C.") {
			continue
		}

		client_info, err := client_info_manager.Get(client_id)
		if err != nil {
			continue
		}

		labels := labeler.GetClientLabels(config_obj, client_id)
		fmt.Printf("Found client %v (%v) labels: %v\n",
			client_id, client_info.Hostname, labels)

		// Now write the new index.
		search.SetIndex(config_obj, client_id, client_id)
		search.SetIndex(config_obj, client_id, "all")
		if client_info.Hostname != "" {
			search.SetIndex(config_obj, client_id, "host:"+client_info.Hostname)
		}

		if client_info.Info.Fqdn != "" {
			search.SetIndex(config_obj, client_id, "host:"+client_info.Info.Fqdn)
		}

		for _, label := range labels {
			if label != "" {
				search.SetIndex(config_obj,
					client_id, "label:"+strings.ToLower(label))
			}
		}
	}
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case index_command_rebuild.FullCommand():
			doRebuildIndex()

		default:
			return false
		}
		return true
	})
}
