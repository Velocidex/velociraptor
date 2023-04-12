package client_info

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/Velocidex/ordereddict"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self ClientInfoManager) GetMetadata(ctx context.Context,
	client_id string) (*ordereddict.Dict, error) {

	client_path_manager := paths.NewClientPathManager(client_id)
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return nil, err
	}

	// If the metadata does not exist - this is not an error we just
	// return a blank one.
	result := &api_proto.ClientMetadata{}
	err = db.GetSubject(self.config_obj,
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

func (self ClientInfoManager) SetMetadata(ctx context.Context,
	client_id string, metadata *ordereddict.Dict, principal string) error {

	existing_metadata, err := self.GetMetadata(ctx, client_id)
	if err != nil {
		return err
	}

	// Merge the new keys with the existing metdata
	updated_keys := []string{}
	for _, key := range metadata.Keys() {
		value_any, _ := metadata.Get(key)
		if utils.IsNil(value_any) {
			updated_keys = append(updated_keys, key)
			existing_metadata.Set(key, nil)
			continue
		}

		value, ok := value_any.(string)
		if !ok {
			value = utils.ToString(value_any)
		}

		existing_value, pres := existing_metadata.GetString(key)
		// Update the key is it is not there, or if it is different
		// from the existing value.
		if !pres || existing_value != value {
			updated_keys = append(updated_keys, key)
			existing_metadata.Update(key, value)
		}
	}

	// Nothing to do here...
	if len(updated_keys) == 0 {
		return nil
	}

	client_path_manager := paths.NewClientPathManager(client_id)
	db, err := datastore.GetDB(self.config_obj)
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

	err = db.SetSubject(self.config_obj,
		client_path_manager.Metadata(), result)
	if err != nil {
		return err
	}

	services.LogAudit(ctx,
		self.config_obj, principal, "SetMetadata",
		ordereddict.NewDict().
			Set("updated_keys", updated_keys).
			Set("client_id", client_id))

	// Notify the changes and log them.
	journal, err := services.GetJournal(self.config_obj)
	if err != nil {
		return err
	}

	return journal.PushRowsToArtifact(ctx, self.config_obj,
		[]*ordereddict.Dict{
			ordereddict.NewDict().
				Set("principal", principal).
				Set("client_id", client_id).
				Set("updated_keys", updated_keys),
		}, "Server.Internal.MetadataModifications", "server", "")
}
