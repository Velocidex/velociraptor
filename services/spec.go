package services

import (
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func MinionServicesSpec() *config_proto.ServerServicesConfig {
	return &config_proto.ServerServicesConfig{
		HuntDispatcher:    true,
		StatsCollector:    true,
		ClientMonitoring:  true,
		SanityChecker:     true,
		FrontendServer:    true,
		MonitoringService: true,
		DynDns:            true,
	}
}

func AllServicesSpec() *config_proto.ServerServicesConfig {
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
	}
}
