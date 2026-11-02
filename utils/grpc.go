package utils

import (
	"context"
	"errors"
	"io"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

var (
	RPC_BACKOFF = 10.0
	RPC_RETRY   = 10
)

type RPCError struct {
	error
	inner error
	s     *status.Status
}

func (self *RPCError) Unwrap() error {
	return self.inner
}

func (self *RPCError) GRPCStatus() *status.Status {
	return self.s
}

func (self *RPCError) Error() string {
	return self.error.Error()
}

// Convert a gRPC Status error into a standard error. This allows
// callers to test for the type of error.
func StatusToError(err error) error {
	status, ok := status.FromError(err)
	if !ok {
		return err
	}

	var perm error
	switch status.Code() {
	case codes.PermissionDenied:
		perm = PermissionDenied

	case codes.InvalidArgument:
		perm = InvalidStatus

	case codes.NotFound:
		perm = NotFoundError

	case codes.Unavailable:
		perm = IOError

	default:
		return err
	}
	return &RPCError{err, perm, status}
}

func RPCTimeout(config_obj *config_proto.Config) time.Duration {
	if config_obj.Datastore == nil ||
		config_obj.Datastore.RemoteDatastoreRpcDeadline == 0 {
		return time.Duration(100 * time.Second)
	}
	return time.Duration(config_obj.Datastore.RemoteDatastoreRpcDeadline) * time.Second
}

func GrpcRetry(ctx context.Context,
	config_obj *config_proto.Config,
	failure_cb func(err error),
	cb func() error,
	logger func(format string, v ...interface{})) error {
	var err error

	for i := 0; i < RPC_RETRY; i++ {
		err = cb()
		if err == nil {
			return nil
		}

		if !isErrorRetriable(err) {
			failure_cb(err)
			return err
		}

		logger("While connecting to remote datastore (retry %v): %v",
			i, err.Error())
		select {
		case <-ctx.Done():
			return timeoutError

		case <-time.After(time.Duration(RPC_BACKOFF) * time.Second):
		}
	}
	return err
}

func isErrorRetriable(err error) bool {

	// Figure out if the error is retryable - only some errors
	// mean a retry is appropriate (see
	// https://pkg.go.dev/google.golang.org/grpc/codes)
	st, ok := status.FromError(err)
	if !ok {
		return false
	}

	switch st.Code() {
	case codes.ResourceExhausted:
		return false

	case codes.DeadlineExceeded, codes.Aborted, codes.Unavailable:
		return true
	default:
		return false
	}
}

func MakeError(err error) *api_proto.Error {
	code := api_proto.ErrorCode_OK
	str := ""

	if errors.Is(err, io.EOF) {
		code = api_proto.ErrorCode_EOF
		str = err.Error()
	} else if err != nil {
		code = api_proto.ErrorCode_ERROR
		str = err.Error()
	}

	return &api_proto.Error{
		Code: code,
		Str:  str,
	}
}

func GetError(err *api_proto.Error) error {
	switch err.Code {
	case api_proto.ErrorCode_OK:
		return nil
	case api_proto.ErrorCode_EOF:
		return io.EOF
	case api_proto.ErrorCode_ERROR:
		return UnknownError
	}
	return UnknownError
}
