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
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self ClientInfoManager) GetMetadata(ctx context.Context,
	client_id string) (*ordereddict.Dict, error) {

	client_path_manager := paths.NewClientPathManager(client_id)
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return nil, err
	}

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
	client_id string, metadata *ordereddict.Dict) error {

	existing_metadata, err := self.GetMetadata(ctx, client_id)
	if err != nil {
		return err
	}

	existing_metadata.MergeFrom(metadata)

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

	return db.SetSubject(self.config_obj,
		client_path_manager.Metadata(), result)
}
