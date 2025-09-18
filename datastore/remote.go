// Implements a remote datastore

package datastore

import (
	"context"
	"errors"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	remote_mu             sync.Mutex
	remote_datastopre_imp = NewRemoteDataStore(context.Background())
	RPC_BACKOFF           = 10.0
	RPC_RETRY             = 10
	timeoutError          = errors.New("gRPC Timeout in Remote datastore")
)

func RPCTimeout(config_obj *config_proto.Config) time.Duration {
	if config_obj.Datastore == nil ||
		config_obj.Datastore.RemoteDatastoreRpcDeadline == 0 {
		return time.Duration(100 * time.Second)
	}
	return time.Duration(config_obj.Datastore.RemoteDatastoreRpcDeadline) * time.Second
}

func Retry(ctx context.Context,
	config_obj *config_proto.Config, cb func() error) error {
	var err error

	for i := 0; i < RPC_RETRY; i++ {
		err = cb()
		if err == nil {
			return nil
		}

		// Figure out if the error is retryable - only some errors
		// mean a retry is appropriate (see
		// https://pkg.go.dev/google.golang.org/grpc/codes)
		st, ok := status.FromError(err)
		if !ok {
			return err
		}

		switch st.Code() {

		// These ones are retryable errors - sleep a bit and retry
		// again.
		case codes.DeadlineExceeded, codes.ResourceExhausted,
			codes.Aborted, codes.Unavailable:
			logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
			logger.Error("While connecting to remote datastore: %v", err)
			select {
			case <-ctx.Done():
				return timeoutError

			case <-time.After(time.Duration(RPC_BACKOFF) * time.Second):
			}

		case codes.Internal, codes.Unknown:
			return err

		default:
			return err
		}
	}
	return err
}

type RemoteDataStore struct {
	ctx context.Context
}

func (self *RemoteDataStore) GetSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec,
	message proto.Message) error {
	return Retry(self.ctx, config_obj, func() error {
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
		context.Background(), RPCTimeout(config_obj), timeoutError)
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

	// Make sure to always call the completion regardless of error
	// paths.
	defer func() {
		if completion != nil &&
			!utils.CompareFuncs(completion, utils.SyncCompleter) {
			completion()
		}
	}()

	return Retry(self.ctx, config_obj, func() error {
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
		context.Background(), RPCTimeout(config_obj), timeoutError)
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

	return err
}

func (self *RemoteDataStore) DeleteSubjectWithCompletion(
	config_obj *config_proto.Config,
	urn api.DSPathSpec, completion func()) error {
	return Retry(self.ctx, config_obj, func() error {
		return self._DeleteSubjectWithCompletion(config_obj, urn, completion)
	})
}

func (self *RemoteDataStore) _DeleteSubjectWithCompletion(
	config_obj *config_proto.Config,
	urn api.DSPathSpec, completion func()) (err error) {

	defer Instrument("delete", "RemoteDataStore", urn)()

	ctx, cancel := utils.WithTimeoutCause(
		context.Background(), RPCTimeout(config_obj), timeoutError)
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

	return err
}

func (self *RemoteDataStore) DeleteSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) error {
	return Retry(self.ctx, config_obj, func() error {
		return self._DeleteSubject(config_obj, urn)
	})
}

func (self *RemoteDataStore) _DeleteSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) (err error) {

	defer Instrument("delete", "RemoteDataStore", urn)()

	ctx, cancel := utils.WithTimeoutCause(
		context.Background(), RPCTimeout(config_obj), timeoutError)
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

	return err
}

func (self *RemoteDataStore) ListChildren(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) ([]api.DSPathSpec, error) {

	var result []api.DSPathSpec
	var err error

	err = Retry(self.ctx, config_obj, func() error {
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
		context.Background(), RPCTimeout(config_obj), timeoutError)
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
		// just means we dont want to use the remote datastore.
		return nil
	}

	if implementation == "RemoteFileDataStore" {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Info("<green>Starting</> remote datastore service")
		remote_mu.Lock()
		remote_datastopre_imp = NewRemoteDataStore(ctx)
		g_impl = nil
		remote_mu.Unlock()

	} else if implementation == "FileBaseDataStore" {
		return startFullDiskChecker(ctx, wg, config_obj)
	}
	return nil
}
