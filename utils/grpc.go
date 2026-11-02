package utils

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/kaptinlin/messageformat-go/pkg/logger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

const (
	RPC_BACKOFF = 10.0
	RPC_RETRY   = 10
)

// Convert a gRPC Status error into a standard error. This allows
// callers to test for the type of error.
func StatusToError(err error) error {
	status, ok := status.FromError(err)
	if !ok {
		return err
	}

	switch status.Code() {
	case codes.PermissionDenied:
		return Wrap(PermissionDenied, "%v", err)

	case codes.InvalidArgument:
		return Wrap(InvalidStatus, "%v", err)

	case codes.NotFound:
		return Wrap(NotFoundError, "%v", err)

	case codes.Unavailable:
		return Wrap(IOError, "%v", err)

	default:
		return err
	}
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
	cb func() error) error {
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

		logger.Error("While connecting to remote datastore (retry %v): %v", i, err)
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
