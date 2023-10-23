package sanity

import (
	"context"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

func (self *SanityChecks) CheckForMinionSettings(
	ctx context.Context, config_obj *config_proto.Config) error {
	if services.IsMaster(config_obj) {
		return nil
	}

	if config_obj.Minion == nil {
		return nil
	}

	if config_obj.Defaults == nil {
		config_obj.Defaults = &config_proto.Defaults{}
	}

	if config_obj.Minion.NotebookNumberOfLocalWorkers > 0 {
		config_obj.Defaults.NotebookNumberOfLocalWorkers = config_obj.Minion.NotebookNumberOfLocalWorkers
	}

	// By default minion workers have lower priority if not specified
	// to keep the default running notebook processing on the master.
	if config_obj.Minion.NotebookWorkerPriority != 0 {
		config_obj.Defaults.NotebookWorkerPriority = config_obj.Minion.NotebookWorkerPriority
	} else {
		config_obj.Defaults.NotebookWorkerPriority = -10
	}

	return nil
}
