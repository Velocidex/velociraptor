package client_info

/*
   Client metadata are arbitrary key/value pairs stored about a
   client.

   There are two types of metadata:

   * Indexed metadata is searchable via the client search. The key of
     the indexed metadata is used as a verb in the search bar. For *
     example `foo:bar` will search for a metadata key of foo with *
     value of bar.

     Indexed metadata fields are specified in the
     Defaults.indexed_client_metadata field in the configuration file.

   * Non-indexed metadata is not indexed but is still associated with
     the client record.

   We assume the total metadata record is fairly small and therefore
   write it into the datastore atomically.

   ## Internal organization

   The indexed fields are stored with the client info record as they
   can be written into the snapshot.

   ClientInfo.Metadata - contains only the indexed k/v pairs in an
      unordered map.

   ClientInfo.AllMetadata - an ordered map or k/v pairs read/written
     from the metadata datastore object.

*/

import (
	"context"
	"errors"
	"os"

	"github.com/Velocidex/ordereddict"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
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

	return self._GetMetadata(ctx, config_obj, client_id)
}

func (self *Store) _GetMetadata(ctx context.Context,
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

func (self *ClientInfoManager) ModifyMetadata(
	ctx context.Context, config_obj *config_proto.Config,
	client_id, principal string, cb func(*ordereddict.Dict) (
		*ordereddict.Dict, error)) error {

	updated_keys := []string{}

	err := self.storage.ModifyMetadata(ctx, config_obj, client_id,
		func(metadata *ordereddict.Dict) (*ordereddict.Dict, error) {
			existing := metadata.Copy()
			new_metadata, err := cb(metadata)
			if err != nil {
				return nil, err
			}

			// calculate the differences
			for _, item := range new_metadata.Items() {
				existing_value, pres := existing.GetString(item.Key)
				if !pres {
					updated_keys = append(updated_keys, item.Key)
					continue
				}

				value := utils.ToString(item.Value)
				if value != existing_value {
					updated_keys = append(updated_keys, item.Key)
				}
				existing.Delete(item.Key)
			}

			for _, key := range existing.Keys() {
				updated_keys = append(updated_keys, key)
			}

			return new_metadata, nil
		})
	if err != nil {
		return err
	}

	// Now audit which fields were modified.
	return auditMetadataChange(
		ctx, config_obj, client_id, principal, updated_keys)
}

// Modify the metadata atomically. This avoids the get/modify/set
// pattern and is race free.
func (self *Store) ModifyMetadata(
	ctx context.Context, config_obj *config_proto.Config,
	client_id string, cb func(*ordereddict.Dict) (
		*ordereddict.Dict, error)) error {

	self.mu.Lock()
	defer self.mu.Unlock()

	// Optionally update the index service.
	indexer, err := services.GetIndexer(config_obj)
	if err != nil {
		return err
	}

	indexed_fields := make(map[string]bool)
	if config_obj.Defaults != nil {
		for _, k := range config_obj.Defaults.IndexedClientMetadata {
			indexed_fields[k] = true
		}
	}

	// Record contains all fields
	record, err := self._GetMetadata(ctx, config_obj, client_id)
	if err != nil {
		return err
	}

	new_metadata, err := cb(record)
	if err != nil {
		return err
	}

	// Nothing to update
	if new_metadata == nil {
		return nil
	}

	// Modify the client record
	client_info, err := self._GetRecord(client_id)
	if err != nil {
		return err
	}

	// The client_info.Metadata only contains indexed fields
	if client_info.Metadata != nil {
		// Unindex all the existing fields
		for k, v := range client_info.Metadata {
			_ = indexer.UnsetIndex(client_id, k+":"+v)
		}
	}

	// Clear the indexed record and start again.
	client_info.Metadata = make(map[string]string)
	stored_obj := &api_proto.ClientMetadata{ClientId: client_id}

	for _, item := range new_metadata.Items() {
		key := item.Key
		if key == "client_id" || key == "metadata" {
			continue
		}

		if utils.IsNil(item.Value) {
			continue
		}

		value := utils.ToString(item.Value)
		if indexed_fields[item.Key] {
			client_info.Metadata[key] = value
			_ = indexer.SetIndex(client_id, key+":"+value)
		}

		stored_obj.Items = append(stored_obj.
			Items, &api_proto.ClientMetadataItem{
			Key: key, Value: value})
	}

	// Set the modified record
	err = self._SetRecord(config_obj, client_info)
	if err != nil {
		return err
	}

	// Now store the full metadata dict in the data store. FIXME: We
	// are stil holding the lock and the below may take a long
	// time....
	client_path_manager := paths.NewClientPathManager(client_id)
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	return db.SetSubject(config_obj,
		client_path_manager.Metadata(), stored_obj)
}

func (self *Store) SetMetadata(
	ctx context.Context, config_obj *config_proto.Config,
	client_id string, metadata *ordereddict.Dict, principal string) error {

	updated_keys := []string{}

	// Merge the new metadata into the existing metadata
	err := self.ModifyMetadata(ctx, config_obj, client_id,
		func(existing_metadata *ordereddict.Dict) (*ordereddict.Dict, error) {

			// Merge the new keys with the existing metadata
			for _, item := range metadata.Items() {
				key := item.Key

				// Nill value means to remove the key, but we only
				// care if the field already is set.
				if utils.IsNil(item.Value) {
					updated_keys = append(updated_keys, key)
					existing_metadata.Delete(key)
					continue
				}

				old_value, _ := existing_metadata.GetString(key)

				// Only update the field if the value is changed.
				value := utils.ToString(item.Value)
				if old_value != value {
					updated_keys = append(updated_keys, key)
				}
				existing_metadata.Set(key, value)

			}

			// If not fields were actually updated, then do nothing.
			if len(updated_keys) == 0 {
				return nil, nil
			}

			// Update to new metadata.
			return existing_metadata, nil
		})

	if err != nil {
		return err
	}

	return auditMetadataChange(
		ctx, config_obj, client_id, principal, updated_keys)
}

func auditMetadataChange(
	ctx context.Context, config_obj *config_proto.Config,
	client_id string, principal string, updated_keys []string) error {

	// Nothing to do here...
	if len(updated_keys) == 0 {
		return nil
	}

	// Generate an audit log
	err := services.LogAudit(ctx,
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
		}, artifacts.CLIENT_METADATA_MODIFICATION)
}
