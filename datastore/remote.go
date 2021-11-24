// Implements a remote datastore

package datastore

import (
	"context"
	"sync"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/grpc_client"
)

var (
	remote_datastopre_imp = NewRemoteDataStore()
	RPC_TIMEOUT           = 100 // Seconds
)

type RemoteDataStore struct{}

func (self *RemoteDataStore) GetSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec,
	message proto.Message) error {

	defer Instrument("read", "RemoteDataStore", urn)()

	ctx, cancel := context.WithTimeout(context.Background(),
		time.Duration(RPC_TIMEOUT)*time.Second)
	defer cancel()

	conn, closer, err := grpc_client.Factory.GetAPIClient(ctx, config_obj)
	defer closer()

	result, err := conn.GetSubject(ctx, &api_proto.DataRequest{
		Pathspec: &api_proto.DSPathSpec{
			Components: urn.Components(),
			PathType:   int64(urn.Type()),
			Tag:        urn.Tag(),
		}})

	if err != nil {
		return err
	}

	serialized_content := result.Data
	if len(serialized_content) == 0 {
		return nil
	}

	// It is really a JSON blob
	if serialized_content[0] == '{' {
		err = protojson.Unmarshal(serialized_content, message)
	} else {
		err = proto.Unmarshal(serialized_content, message)
	}

	return err
}

// Write the data synchronously.
func (self *RemoteDataStore) SetSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec,
	message proto.Message) error {

	wg := sync.WaitGroup{}
	defer wg.Wait()

	wg.Add(1)
	return self.SetSubjectWithCompletion(config_obj, urn, message, wg.Done)
}

func (self *RemoteDataStore) SetSubjectWithCompletion(
	config_obj *config_proto.Config,
	urn api.DSPathSpec,
	message proto.Message,
	completion func()) error {

	defer Instrument("write", "RemoteDataStore", urn)()

	var value []byte
	var err error

	if urn.Type() == api.PATH_TYPE_DATASTORE_JSON {
		value, err = protojson.Marshal(message)
		if err != nil {
			return err
		}
	} else {
		value, err = proto.Marshal(message)
	}

	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(),
		time.Duration(RPC_TIMEOUT)*time.Second)
	defer cancel()

	conn, closer, err := grpc_client.Factory.GetAPIClient(ctx, config_obj)
	defer closer()

	_, err = conn.SetSubject(ctx, &api_proto.DataRequest{
		Data: value,
		Sync: completion != nil,
		Pathspec: &api_proto.DSPathSpec{
			Components: urn.Components(),
			PathType:   int64(urn.Type()),
			Tag:        urn.Tag(),
		}})

	if completion != nil {
		completion()
	}

	return err
}

func (self *RemoteDataStore) DeleteSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) error {
	defer Instrument("delete", "RemoteDataStore", urn)()

	ctx, cancel := context.WithTimeout(context.Background(),
		time.Duration(RPC_TIMEOUT)*time.Second)
	defer cancel()

	conn, closer, err := grpc_client.Factory.GetAPIClient(ctx, config_obj)
	defer closer()

	_, err = conn.DeleteSubject(ctx, &api_proto.DataRequest{
		Pathspec: &api_proto.DSPathSpec{
			Components: urn.Components(),
			PathType:   int64(urn.Type()),
			Tag:        urn.Tag(),
		}})

	return err
}

// Lists all the children of a URN.
func (self *RemoteDataStore) ListChildren(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) ([]api.DSPathSpec, error) {

	defer Instrument("list", "RemoteDataStore", urn)()

	ctx, cancel := context.WithTimeout(context.Background(),
		time.Duration(RPC_TIMEOUT)*time.Second)
	defer cancel()

	conn, closer, err := grpc_client.Factory.GetAPIClient(ctx, config_obj)
	defer closer()

	result, err := conn.ListChildren(ctx, &api_proto.DataRequest{
		Pathspec: &api_proto.DSPathSpec{
			Components: urn.Components(),
			PathType:   int64(urn.Type()),
			Tag:        urn.Tag(),
		}})

	if err != nil {
		return nil, err
	}

	children := make([]api.DSPathSpec, 0, len(result.Children))
	for _, child := range result.Children {
		child_urn := path_specs.NewUnsafeDatastorePath(
			child.Components...).SetType(api.PathType(child.PathType))
		if child.IsDir {
			child_urn = child_urn.SetDir()
		}
		children = append(children, child_urn)
	}

	return children, err
}

// Called to close all db handles etc. Not thread safe.
func (self *RemoteDataStore) Close() {}
func (self *RemoteDataStore) Debug(config_obj *config_proto.Config) {
}

func NewRemoteDataStore() *RemoteDataStore {
	result := &RemoteDataStore{}
	return result
}
