// +build !linux

package datastore

import (
	"context"
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func startFullDiskChecker(ctx context.Context, wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {
	return nil
}
