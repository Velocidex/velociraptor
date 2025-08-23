package client_info

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/Velocidex/ordereddict"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self *ClientInfoManager) GetMetadata(ctx context.Context,
	client_id string) (*ordereddict.Dict, error) {
	return self.storage.GetMetadata(ctx, self.config_obj, client_id)
}

func (self *ClientInfoManager) SetMetadata(ctx context.Context,
	client_id string, metadata *ordereddict.Dict, principal string) error {
	return self.storage.SetMetadata(ctx, self.config_obj,
		client_id, metadata, principal)
}

func (self *Store) GetMetadata(ctx context.Context,
	config_obj *config_proto.Config,
	client_id string) (*ordereddict.Dict, error) {

	client_path_manager := paths.NewClientPathManager(client_id)
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	// If the metadata does not exist - this is not an error we just
	// return a blank one.
	result := &api_proto.ClientMetadata{}
	err = db.GetSubject(config_obj,
		client_path_manager.Metadata(), result)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	result_dict := ordereddict.NewDict()
	for _, item := range result.Items {
		result_dict.Set(item.Key, item.Value)
	}
	return result_dict, nil
}

func (self *Store) SetMetadata(
	ctx context.Context, config_obj *config_proto.Config,
	client_id string, metadata *ordereddict.Dict, principal string) error {

	existing_metadata, err := self.GetMetadata(ctx, config_obj, client_id)
	if err != nil {
		return err
	}

	// Merge the new keys with the existing metdata
	updated_keys := []string{}
	for _, item := range metadata.Items() {
		if utils.IsNil(item.Value) {
			updated_keys = append(updated_keys, item.Key)
			existing_metadata.Set(item.Key, nil)
			continue
		}

		value, ok := item.Value.(string)
		if !ok {
			value = utils.ToString(item.Value)
		}

		existing_value, pres := existing_metadata.GetString(item.Key)
		// Update the key is it is not there, or if it is different
		// from the existing value.
		if !pres || existing_value != value {
			updated_keys = append(updated_keys, item.Key)
			existing_metadata.Update(item.Key, value)
		}
	}

	// Nothing to do here...
	if len(updated_keys) == 0 {
		return nil
	}

	// Here existing_metadata will contain a merged old vs new
	// metadata dict and should be ready to store.  We can extract
	// some of the metadata into the client info index.
	err = self.updateClientMetadataIndex(
		ctx, config_obj, client_id, existing_metadata)
	if err != nil {
		return err
	}

	client_path_manager := paths.NewClientPathManager(client_id)
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	result := &api_proto.ClientMetadata{ClientId: client_id}
	for _, key := range existing_metadata.Keys() {
		if key == "client_id" || key == "metadata" {
			continue
		}

		value, pres := existing_metadata.GetString(key)
		if !pres {
			// Users can set a parameter to NULL to make it disappear.
			value_any, _ := existing_metadata.Get(key)
			if utils.IsNil(value_any) {
				continue
			}
			value = fmt.Sprintf("%v", value_any)
		}

		result.Items = append(result.Items, &api_proto.ClientMetadataItem{
			Key: key, Value: value})
	}

	err = db.SetSubject(config_obj,
		client_path_manager.Metadata(), result)
	if err != nil {
		return err
	}

	err = services.LogAudit(ctx,
		config_obj, principal, "SetMetadata",
		ordereddict.NewDict().
			Set("updated_keys", updated_keys).
			Set("client_id", client_id))
	if err != nil {
		return err
	}

	// Notify the changes and log them.
	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return err
	}

	return journal.PushRowsToArtifact(ctx, config_obj,
		[]*ordereddict.Dict{
			ordereddict.NewDict().
				Set("principal", principal).
				Set("client_id", client_id).
				Set("updated_keys", updated_keys),
		}, "Server.Internal.MetadataModifications", "server", "")
}
