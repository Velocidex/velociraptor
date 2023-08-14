// +build linux

package datastore

import (
	"context"
	"sync"
	"syscall"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
)

func AvailableDiskSpace(
	db DataStore, config_obj *config_proto.Config) (uint64, error) {
	var stat syscall.Statfs_t
	syscall.Statfs(config_obj.Datastore.Location, &stat)

	available := stat.Bavail * uint64(stat.Bsize)

	min_allowed_file_space_mb := uint64(config_obj.Datastore.MinAllowedFileSpaceMb)
	if min_allowed_file_space_mb == 0 {
		// We need at least 50mb by default.
		min_allowed_file_space_mb = 50
	}

	filebased_db, ok := db.(*FileBaseDataStore)
	if ok {
		// If we have insufficient disk space, set the filestore to
		// stop writing.
		if available/1024/1024 < min_allowed_file_space_mb {
			logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
			logger.Error("FileBaseDataStore: Insufficient free disk space! We need at least %v Mb but we have %v!. Disabling write operations to avoid file corruption. Free some disk space or grow the partition.",
				min_allowed_file_space_mb, available/1024/1024)

			// Stop writing - disk is full!
			filebased_db.SetError(insufficientDiskSpace)

		} else {
			// Start writing again.
			filebased_db.SetError(nil)
		}
	}
	return available, nil
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
		disk_check_freq = 10
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

		for {
			select {
			case <-ctx.Done():
				return

			case <-time.After(time.Duration(disk_check_freq) * time.Second):
				AvailableDiskSpace(db, config_obj)
			}
		}
	}()

	return nil
}
