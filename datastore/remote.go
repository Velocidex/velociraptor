// Implements a remote datastore

package datastore

import (
	"context"
	"sync"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

var (
	remote_mu             sync.Mutex
	remote_datastopre_imp *RemoteDataStore
)

type RemoteDataStore struct {
	ctx context.Context
}

func (self *RemoteDataStore) GetSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec,
	message proto.Message) error {
	return utils.GrpcRetry(self.ctx, config_obj,
		func(err error) {
			maybeLog(config_obj, urn, "GetSubject", err)
		},
		func() error {
			return self._GetSubject(config_obj, urn, message)
		})
}

func (self *RemoteDataStore) Healthy() error {
	return nil
}

func (self *RemoteDataStore) _GetSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec,
	message proto.Message) (err error) {

	defer Instrument("read", "RemoteDataStore", urn)()

	ctx, cancel := utils.WithTimeoutCause(
		self.ctx, utils.RPCTimeout(config_obj), utils.TimeoutError)
	defer cancel()

	// Make the call as the superuser
	conn, closer, err := grpc_client.Factory.GetAPIClient(ctx,
		grpc_client.SuperUser, config_obj)
	if err != nil {
		return err
	}
	defer func() {
		err1 := closer()
		if err1 != nil && err == nil {
			err = err1
		}
	}()

	result, err := conn.GetSubject(ctx, &api_proto.DataRequest{
		OrgId: config_obj.OrgId,
		Pathspec: &api_proto.DSPathSpec{
			Components: urn.Components(),
			PathType:   int64(urn.Type()),
			Tag:        urn.Tag(),
		}})

	if err != nil {
		return utils.StatusToError(err)
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

	// Make sure to always call the completion regardless of error
	// paths.
	defer func() {
		if completion != nil &&
			!utils.CompareFuncs(completion, utils.SyncCompleter) {
			completion()
		}
	}()

	return utils.GrpcRetry(self.ctx, config_obj,
		func(err error) {
			maybeLog(config_obj, urn, "GetSubject", err)
		},
		func() error {
			return self._SetSubjectWithCompletion(
				config_obj, urn, message, completion)
		})
}

func (self *RemoteDataStore) _SetSubjectWithCompletion(
	config_obj *config_proto.Config,
	urn api.DSPathSpec,
	message proto.Message,
	completion func()) (err error) {

	defer Instrument("write", "RemoteDataStore", urn)()

	var value []byte

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

	ctx, cancel := utils.WithTimeoutCause(
		self.ctx, utils.RPCTimeout(config_obj), utils.TimeoutError)
	defer cancel()

	// Make the call as the superuser
	conn, closer, err := grpc_client.Factory.GetAPIClient(
		ctx, grpc_client.SuperUser, config_obj)
	if err != nil {
		return err
	}
	defer func() {
		err1 := closer()
		if err1 != nil && err == nil {
			err = err1
		}
	}()

	_, err = conn.SetSubject(ctx, &api_proto.DataRequest{
		OrgId: config_obj.OrgId,
		Data:  value,
		Sync:  completion != nil,
		Pathspec: &api_proto.DSPathSpec{
			Components: urn.Components(),
			PathType:   int64(urn.Type()),
			Tag:        urn.Tag(),
		}})

	return utils.StatusToError(err)
}

func (self *RemoteDataStore) Stat(config_obj *config_proto.Config,
	urn api.DSPathSpec) (api.FileInfo, error) {
	defer Instrument("read", "RemoteDataStore", urn)()

	ctx, cancel := utils.WithTimeoutCause(
		self.ctx, utils.RPCTimeout(config_obj), utils.TimeoutError)
	defer cancel()

	// Make the call as the superuser
	conn, closer, err := grpc_client.Factory.GetAPIClient(ctx,
		grpc_client.SuperUser, config_obj)
	if err != nil {
		return nil, err
	}
	defer func() {
		err1 := closer()
		if err1 != nil && err == nil {
			err = err1
		}
	}()

	result, err := conn.GetSubject(ctx, &api_proto.DataRequest{
		OrgId: config_obj.OrgId,
		Pathspec: &api_proto.DSPathSpec{
			Components: urn.Components(),
			PathType:   int64(urn.Type()),
			Tag:        urn.Tag(),
		}})

	if err != nil {
		return nil, utils.StatusToError(err)
	}

	return &vtesting.MockFileInfo{
		Name_:     urn.Base(),
		PathSpec_: urn.AsFilestorePath(),
		FullPath_: urn.String(),
		Mode_:     0664,
		Size_:     int64(len(result.Data)),
	}, nil

}

func (self *RemoteDataStore) DeleteSubjectWithCompletion(
	config_obj *config_proto.Config,
	urn api.DSPathSpec, completion func()) error {
	return utils.GrpcRetry(self.ctx, config_obj,
		func(err error) {
			maybeLog(config_obj, urn, "DeleteSubjectWithCompletion", err)
		},
		func() error {
			return self._DeleteSubjectWithCompletion(config_obj, urn, completion)
		})
}

func (self *RemoteDataStore) _DeleteSubjectWithCompletion(
	config_obj *config_proto.Config,
	urn api.DSPathSpec, completion func()) (err error) {

	defer Instrument("delete", "RemoteDataStore", urn)()

	ctx, cancel := utils.WithTimeoutCause(
		self.ctx, utils.RPCTimeout(config_obj), utils.TimeoutError)
	defer cancel()

	conn, closer, err := grpc_client.Factory.GetAPIClient(
		ctx, grpc_client.SuperUser, config_obj)
	if err != nil {
		return err
	}
	defer func() {
		err1 := closer()
		if err1 != nil && err == nil {
			err = err1
		}
	}()

	_, err = conn.DeleteSubject(ctx, &api_proto.DataRequest{
		OrgId: config_obj.OrgId,
		Sync:  completion != nil,
		Pathspec: &api_proto.DSPathSpec{
			Components: urn.Components(),
			PathType:   int64(urn.Type()),
			Tag:        urn.Tag(),
		}})

	if completion != nil &&
		!utils.CompareFuncs(completion, utils.SyncCompleter) {
		completion()
	}

	return utils.StatusToError(err)
}

func (self *RemoteDataStore) DeleteSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) error {
	return utils.GrpcRetry(self.ctx, config_obj,
		func(err error) {
			maybeLog(config_obj, urn, "DeleteSubject", err)
		},
		func() error {
			return self._DeleteSubject(config_obj, urn)
		})
}

func (self *RemoteDataStore) _DeleteSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) (err error) {

	defer Instrument("delete", "RemoteDataStore", urn)()

	ctx, cancel := utils.WithTimeoutCause(
		self.ctx, utils.RPCTimeout(config_obj), utils.TimeoutError)
	defer cancel()

	conn, closer, err := grpc_client.Factory.GetAPIClient(
		ctx, grpc_client.SuperUser, config_obj)
	if err != nil {
		return err
	}
	defer func() {
		err1 := closer()
		if err1 != nil && err == nil {
			err = err1
		}
	}()

	_, err = conn.DeleteSubject(ctx, &api_proto.DataRequest{
		OrgId: config_obj.OrgId,
		Pathspec: &api_proto.DSPathSpec{
			Components: urn.Components(),
			PathType:   int64(urn.Type()),
			Tag:        urn.Tag(),
		}})

	return utils.StatusToError(err)
}

func (self *RemoteDataStore) ListChildren(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) ([]api.DSPathSpec, error) {

	var result []api.DSPathSpec
	var err error

	err = utils.GrpcRetry(self.ctx, config_obj,
		func(err error) {
			maybeLog(config_obj, urn, "ListChildren", err)
		},
		func() error {
			result, err = self._ListChildren(config_obj, urn)
			return err
		})

	return result, err
}

// Lists all the children of a URN.
func (self *RemoteDataStore) _ListChildren(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) (res []api.DSPathSpec, err error) {

	defer Instrument("list", "RemoteDataStore", urn)()

	ctx, cancel := utils.WithTimeoutCause(
		self.ctx, utils.RPCTimeout(config_obj), utils.TimeoutError)
	defer cancel()

	conn, closer, err := grpc_client.Factory.GetAPIClient(
		ctx, grpc_client.SuperUser, config_obj)
	if err != nil {
		return nil, err
	}
	defer func() {
		err1 := closer()
		if err1 != nil && err == nil {
			err = err1
		}
	}()

	result, err := conn.ListChildren(ctx, &api_proto.DataRequest{
		OrgId: config_obj.OrgId,
		Pathspec: &api_proto.DSPathSpec{
			Components: urn.Components(),
			PathType:   int64(urn.Type()),
			Tag:        urn.Tag(),
		}})

	if err != nil {
		return nil, utils.StatusToError(err)
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

func NewRemoteDataStore(ctx context.Context) *RemoteDataStore {
	result := &RemoteDataStore{ctx: ctx}
	return result
}

func StartDatastore(
	ctx context.Context, wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	// Initialize the remote datastore if needed.
	implementation, err := GetImplementationName(config_obj)
	if err != nil {
		// Invalid datastore configuration is not an issue here - it
		// just means we don't want to use the remote datastore.
		return nil
	}

	switch implementation {
	// These datastores require starting the remote connection.
	case "RemoteFileDataStore", "MemcacheFileDataStore", "RemoteRPCFileStore":
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Info("<green>Starting</> remote datastore service")
		remote_mu.Lock()
		remote_datastopre_imp = NewRemoteDataStore(ctx)
		g_impl = nil
		remote_mu.Unlock()

	case "FileBaseDataStore":
		return startFullDiskChecker(ctx, wg, config_obj)
	}
	return nil
}

func maybeLog(config_obj *config_proto.Config, urn api.DSPathSpec, op string, err error) {
	if !utils.IsNotFound(err) {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Error("RemoteDataStore: %s %s: %v", op, urn.String(), err)
	}
}
