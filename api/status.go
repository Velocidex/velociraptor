package api

import (
	"os"

	errors "github.com/go-errors/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Convert from various errors into gRPC status errors. This will be
// translated into proper HTTP codes by the gRPC gateway
func Status(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return status.Error(codes.NotFound, err.Error())
	}

	return err
}
