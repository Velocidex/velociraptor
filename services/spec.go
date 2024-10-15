package services

import (
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

// The Golden test harness starts all services except for the sanity service.
func GoldenServicesSpec() *config_proto.ServerServicesConfig {
	result := AllServerServicesSpec()
	result.SanityChecker = false
	return result
}

func GenericToolServices() *config_proto.ServerServicesConfig {
	return &config_proto.ServerServicesConfig{
		RepositoryManager:   true,
		InventoryService:    true,
		Launcher:            true,
		JournalService:      true,
		UserManager:         true,
		NotificationService: true,
	}
}

func ClientServicesSpec() *config_proto.ServerServicesConfig {
	return &config_proto.ServerServicesConfig{
		JournalService:      true,
		RepositoryManager:   true,
		InventoryService:    true,
		NotificationService: true,
		Launcher:            true,
		HttpCommunicator:    true,
		ClientEventTable:    true,
	}
}

// The minion only runs a small subset of services.
func MinionServicesSpec() *config_proto.ServerServicesConfig {
	return &config_proto.ServerServicesConfig{
		HuntDispatcher:      true,
		StatsCollector:      true,
		ClientMonitoring:    true,
		ClientInfo:          true,
		Label:               true,
		NotificationService: true,
		ReplicationService:  true,
		Launcher:            true,
		RepositoryManager:   true,
		FrontendServer:      true,
		JournalService:      true,
		SchedulerService:    true,
		DynDns:              true,
		InventoryService:    true,

		// Run the notebook service on the minion so it can run
		// notebook jobs remotely.
		NotebookService: true,
		UserManager:     true,
	}
}

// The GUI/Frontend runs all services.
func AllServerServicesSpec() *config_proto.ServerServicesConfig {
	return &config_proto.ServerServicesConfig{
		HuntManager:         true,
		HuntDispatcher:      true,
		StatsCollector:      true,
		ServerMonitoring:    true,
		ServerArtifacts:     true,
		DynDns:              true,
		Interrogation:       true,
		SanityChecker:       true,
		VfsService:          true,
		UserManager:         true,
		ClientMonitoring:    true,
		MonitoringService:   true,
		ApiServer:           true,
		FrontendServer:      true,
		GuiServer:           true,
		IndexServer:         true,
		JournalService:      true,
		NotificationService: true,
		RepositoryManager:   true,
		InventoryService:    true,
		ClientInfo:          true,
		Label:               true,
		Launcher:            true,
		NotebookService:     true,
		SchedulerService:    true,
		BackupService:       true,
	}
}
