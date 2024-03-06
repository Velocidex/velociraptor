package api

import (
	"fmt"
	"os"

	errors "github.com/go-errors/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/utils"
)

// Convert from various errors into gRPC status errors. This will be
// translated into proper HTTP codes by the gRPC gateway
func Status(verbose bool, err error) error {
	// Do not interfer with status messages already.
	_, ok := status.FromError(err)
	if ok {
		return err
	}

	// Do not report this error.
	if err == utils.InlineError {
		return nil
	}

	if errors.Is(err, acls.PermissionDenied) {
		return status.Error(codes.PermissionDenied, err.Error())
	}

	if errors.Is(err, utils.InvalidStatus) {
		return status.Error(codes.InvalidArgument, err.Error())
	}

	// With the verbose flag give more detailed errors to the browser.
	if verbose {
		if errors.Is(err, os.ErrNotExist) {
			return status.Error(codes.NotFound, err.Error())
		}

		return err
	}

	// In production provide generic errors.
	if errors.Is(err, os.ErrNotExist) {
		return status.Error(codes.NotFound, "Not Found")
	}

	// TODO: For now unknown errors will be returned to the user, but
	// we need to tighten it here to prevent internal information
	// leak.
	return status.Error(codes.Unavailable, err.Error())
}

func InvalidStatus(message string) error {
	return fmt.Errorf("%w: %s", utils.InvalidStatus, message)
}

func PermissionDenied(err error, message string) error {
	if err != nil {
		return status.Error(codes.PermissionDenied,
			fmt.Sprintf("%v: %v", err, message))
	}
	return status.Error(codes.PermissionDenied, message)
}
