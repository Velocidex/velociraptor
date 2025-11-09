//go:build linux || windows
// +build linux windows

package datastore

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Velocidex/ordereddict"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/psutils"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

var (
	disabledWrites int64 = -1
)

func isDisabled(config_obj *config_proto.Config) bool {
	// Only peresent in debug mode
	if config_obj.DebugMode &&
		atomic.LoadInt64(&disabledWrites) < 0 {

		atomic.StoreInt64(&disabledWrites, 0)

		vql_subsystem.RegisterFunction(
			vfilter.GenericFunction{
				FunctionName: "disable_writes",
				Metadata:     vql_subsystem.VQLMetadata().Build(),
				Function: func(
					ctx context.Context,
					scope vfilter.Scope,
					args *ordereddict.Dict) types.Any {
					ok, _ := args.GetBool("clear")
					if ok {
						atomic.StoreInt64(&disabledWrites, 0)
					} else {
						atomic.StoreInt64(&disabledWrites, 1)
					}
					return true
				}})
	}

	return atomic.LoadInt64(&disabledWrites) > 0
}

func AvailableDiskSpace(
	db DataStore, config_obj *config_proto.Config) (uint64, error) {

	stat, err := psutils.Usage(config_obj.Datastore.Location)
	if err != nil {
		return 0, err
	}

	min_allowed_file_space_mb := uint64(
		config_obj.Datastore.MinAllowedFileSpaceMb)
	if min_allowed_file_space_mb == 0 {
		// We need at least 50mb by default.
		min_allowed_file_space_mb = 50
	}

	free_mb := stat.Free / 1024 / 1024

	filebased_db, ok := db.(*FileBaseDataStore)
	if ok {
		// If we have insufficient disk space, set the filestore to
		// stop writing.
		if isDisabled(config_obj) ||
			free_mb < min_allowed_file_space_mb {
			msg := fmt.Sprintf("FileBaseDataStore: Insufficient free disk space! We need at least %v Mb but we have %v!. Disabling write operations to avoid file corruption. Free some disk space or grow the partition.",
				min_allowed_file_space_mb, free_mb)

			logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
			logger.Error("%v", msg)

			// Stop writing - disk is full!
			filebased_db.SetError(insufficientDiskSpace)

			frontend_service, err := services.GetFrontendManager(config_obj)
			if err == nil {
				frontend_service.SetGlobalMessage(
					&api_proto.GlobalUserMessage{
						Key:     "DiskSpace",
						Level:   "ERROR",
						Message: msg,
					})
			}

		} else {
			// Start writing again.
			filebased_db.SetError(nil)

			frontend_service, err := services.GetFrontendManager(config_obj)
			if err == nil {
				frontend_service.SetGlobalMessage(
					&api_proto.GlobalUserMessage{
						Key: "DiskSpace",
					})
			}
		}
	}
	return stat.Free, nil
}

func startFullDiskChecker(ctx context.Context, wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	if config_obj.Datastore.MinAllowedFileSpaceMb < 0 ||
		config_obj.Datastore.DiskCheckFrequencySec < 0 {
		return nil
	}

	// How often to check the disk is full.
	disk_check_freq := config_obj.Datastore.DiskCheckFrequencySec
	if disk_check_freq == 0 {
		disk_check_freq = 20
	}

	volumePath := ""
	if config_obj.Datastore != nil {
		volumePath = config_obj.Datastore.Location
	}

	if volumePath == "" {
		return nil
	}

	db, err := GetDB(config_obj)
	if err != nil {
		return err
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		_, _ = AvailableDiskSpace(db, config_obj)

		for {
			select {
			case <-ctx.Done():
				return

			case <-time.After(time.Duration(disk_check_freq) * time.Second):
				_, _ = AvailableDiskSpace(db, config_obj)
			}
		}
	}()

	return nil
}
