package sanity

import (
	"context"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/search"
)

func maybeMigrateClientIndex(
	ctx context.Context, config_obj *config_proto.Config) error {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	items, _ := db.ListChildren(config_obj, paths.CLIENT_INDEX_URN)
	if len(items) > 0 {
		return nil
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("Converting legacy client index to new format")

	count := 0

	// Migrate the old index to the new index.
	err = db.Walk(config_obj, paths.CLIENT_INDEX_URN_DEPRECATED,
		func(path api.DSPathSpec) error {
			client_id := path.Base()
			term := path.Dir().Base()
			count++
			if count%500 == 0 {
				logger.Info("Converted %v index items to the new format", count)
			}
			return search.SetIndex(config_obj, client_id, term)
		})

	return err
}
