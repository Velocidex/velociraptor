package main

import (
	"fmt"
	"strings"

	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/search"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/client_info"
)

var (
	index_command = app.Command(
		"index", "Manage client index.")

	index_command_rebuild = index_command.Command(
		"rebuild", "Rebuild client index")
)

func doRebuildIndex() error {
	config_obj, err := makeDefaultConfigLoader().
		WithRequiredFrontend().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	sm, err := startEssentialServices(config_obj)
	if err != nil {
		return fmt.Errorf("Starting services: %w", err)
	}
	defer sm.Close()

	err = sm.Start(client_info.StartClientInfoService)
	if err != nil {
		return fmt.Errorf("Starting services: %w", err)
	}

	err = sm.Start(datastore.StartMemcacheFileService)
	if err != nil {
		return fmt.Errorf("Starting services: %w", err)
	}

	client_info_manager, err := services.GetClientInfoManager()
	if err != nil {
		return fmt.Errorf("Starting services: %w", err)
	}

	labeler := services.GetLabeler()

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	// This will be very slow on EFS or large directories but it is
	// necessary.
	clients, err := db.ListChildren(
		config_obj, paths.NewClientPathManager("X").Path().Dir())
	if err != nil {
		return fmt.Errorf("Enumerating clients: %w", err)
	}

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

		if client_info.Fqdn != "" {
			search.SetIndex(config_obj, client_id, "host:"+client_info.Fqdn)
		}

		for _, label := range labels {
			if label != "" {
				search.SetIndex(config_obj,
					client_id, "label:"+strings.ToLower(label))
			}
		}
	}
	return nil
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case index_command_rebuild.FullCommand():
			FatalIfError(index_command_rebuild, doRebuildIndex)

		default:
			return false
		}
		return true
	})
}
