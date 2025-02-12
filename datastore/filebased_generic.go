//go:build !linux && !windows
// +build !linux,!windows

package datastore

import (
	"context"
	"errors"
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func AvailableDiskSpace(
	db DataStore, config_obj *config_proto.Config) (uint64, error) {
	return 0, errors.New("Not implemented")
}

func startFullDiskChecker(ctx context.Context, wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {
	return nil
}
