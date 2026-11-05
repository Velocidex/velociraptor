package services

import config_proto "www.velocidex.com/golang/velociraptor/config/proto"

func GetLocalWorkers(config_obj *config_proto.Config) int64 {
	local_workers := int64(1)
	if config_obj.Defaults.NotebookNumberOfLocalWorkers != 0 {
		local_workers = config_obj.Defaults.NotebookNumberOfLocalWorkers
	}
	if IsMinion(config_obj) && config_obj.Minion != nil &&
		config_obj.Minion.NotebookNumberOfLocalWorkers > 0 {
		local_workers = config_obj.Minion.NotebookNumberOfLocalWorkers
	}

	return local_workers
}

func GetWorkerPriority(config_obj *config_proto.Config) int {
	var priority int64
	if config_obj.Defaults != nil {
		priority = config_obj.Defaults.NotebookWorkerPriority
	}

	if IsMinion(config_obj) {
		// By default minion workers have lower priority if not
		// specified to keep the default running notebook processing
		// on the master.
		priority = -10
		if config_obj.Minion != nil &&
			config_obj.Minion.NotebookWorkerPriority != 0 {
			priority = config_obj.Minion.NotebookWorkerPriority
		}
	}

	return int(priority)
}
