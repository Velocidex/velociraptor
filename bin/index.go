package main

import (
	"fmt"
	"time"

	"www.velocidex.com/golang/velociraptor/datastore"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/indexing"
	"www.velocidex.com/golang/velociraptor/startup"
)

var (
	index_command = app.Command(
		"index", "Manage client index.")

	index_command_rebuild = index_command.Command(
		"rebuild", "Rebuild client index")
)

func doRebuildIndex() error {
	logging.DisableLogging()

	config_obj, err := makeDefaultConfigLoader().
		WithRequiredUser().
		WithRequiredFrontend().
		LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	config_obj.Services = services.GenericToolServices()
	sm, err := startup.StartToolServices(ctx, config_obj)
	defer sm.Close()

	if err != nil {
		return err
	}

	err = sm.Start(datastore.StartMemcacheFileService)
	if err != nil {
		return fmt.Errorf("Starting services: %w", err)
	}

	now := time.Now()
	path_manager := paths.NewIndexPathManager()
	dest := path_manager.SnapshotTimed()
	fmt.Printf("Writing new index snapshot at %v\n",
		dest.AsFilestoreFilename(config_obj))

	defer func() {
		fmt.Printf("Done in %v\n", time.Now().Sub(now))
	}()

	new_indexer := indexing.NewIndexer(config_obj)
	err = new_indexer.LoadIndexFromDatastore(sm.Ctx, config_obj)
	if err != nil {
		return fmt.Errorf("Building index: %w", err)
	}

	// Write a timed snapshot
	return new_indexer.WriteSnapshot(config_obj, dest)
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
