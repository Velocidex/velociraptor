// A utility to start up all essential services.

package startup

import (
	"fmt"

	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/broadcast"
	"www.velocidex.com/golang/velociraptor/services/client_info"
	"www.velocidex.com/golang/velociraptor/services/client_monitoring"
	"www.velocidex.com/golang/velociraptor/services/ddclient"
	"www.velocidex.com/golang/velociraptor/services/hunt_dispatcher"
	"www.velocidex.com/golang/velociraptor/services/hunt_manager"
	"www.velocidex.com/golang/velociraptor/services/indexing"
	"www.velocidex.com/golang/velociraptor/services/interrogation"
	"www.velocidex.com/golang/velociraptor/services/inventory"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/labels"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/services/notifications"
	"www.velocidex.com/golang/velociraptor/services/repository"
	"www.velocidex.com/golang/velociraptor/services/sanity"
	"www.velocidex.com/golang/velociraptor/services/server_artifacts"
	"www.velocidex.com/golang/velociraptor/services/server_monitoring"
	"www.velocidex.com/golang/velociraptor/services/vfs_service"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func getServerServices(config_obj *config_proto.Config) *config_proto.ServerServicesConfig {
	if config_obj.Frontend == nil {
		return &config_proto.ServerServicesConfig{}
	}

	// If no service specification is set, we start all services
	// on the primary frontend.
	if config_obj.Frontend.ServerServices == nil {
		return &config_proto.ServerServicesConfig{
			HuntManager:       true,
			HuntDispatcher:    true,
			StatsCollector:    true,
			ServerMonitoring:  true,
			ServerArtifacts:   true,
			DynDns:            true,
			Interrogation:     true,
			SanityChecker:     true,
			VfsService:        true,
			UserManager:       true,
			ClientMonitoring:  true,
			MonitoringService: true,
			ApiServer:         true,
			FrontendServer:    true,
			GuiServer:         true,
			IndexServer:       true,
		}
	}

	return config_obj.Frontend.ServerServices
}

func StartupEssentialServices(sm *services.Service) error {
	spec := getServerServices(sm.Config)

	err := sm.Start(datastore.StartRemoteDatastore)
	if err != nil {
		return err
	}

	// Updates DynDNS records if needed. Frontends need to maintain
	// their IP addresses.
	if spec.DynDns {
		err := sm.Start(ddclient.StartDynDNSService)
		if err != nil {
			return err
		}
	}

	j, _ := services.GetJournal()
	if j == nil {
		err := sm.Start(journal.StartJournalService)
		if err != nil {
			return err
		}
	}

	b, _ := services.GetBroadcastService()
	if b == nil {
		err := sm.Start(broadcast.StartBroadcastService)
		if err != nil {
			return err
		}
	}

	if services.GetNotifier() == nil {
		err := sm.Start(notifications.StartNotificationService)
		if err != nil {
			return err
		}
	}

	if services.GetInventory() == nil {
		err := sm.Start(inventory.StartInventoryService)
		if err != nil {
			return err
		}
	}

	manager, _ := services.GetRepositoryManager()
	if manager == nil {
		err := sm.Start(repository.StartRepositoryManager)
		if err != nil {
			return err
		}
	}

	launcher_obj, _ := services.GetLauncher()
	if launcher_obj == nil {
		err := sm.Start(launcher.StartLauncherService)
		if err != nil {
			return err
		}
	}

	if services.GetLabeler() == nil {
		err := sm.Start(labels.StartLabelService)
		if err != nil {
			return err
		}
	}

	return nil
}

// Start usual services that run on frontends only (i.e. not the client).
func StartupFrontendServices(sm *services.Service) error {
	spec := getServerServices(sm.Config)

	err := sm.Start(datastore.StartMemcacheFileService)
	if err != nil {
		return err
	}

	_, err = services.GetClientInfoManager()
	if err != nil {
		err := sm.Start(client_info.StartClientInfoService)
		if err != nil {
			return err
		}
	}

	if spec.IndexServer {
		err = sm.Start(indexing.StartIndexingService)
		if err != nil {
			return err
		}
	}

	// Check everything is ok before we can start.
	if spec.ClientMonitoring {
		// Maintans the client's event monitoring table. All frontends
		// need to follow this so they can propagate changes to
		// clients.
		err := sm.Start(client_monitoring.StartClientMonitoringService)
		if err != nil {
			return err
		}
	}

	if spec.SanityChecker {
		err := sm.Start(sanity.StartSanityCheckService)
		if err != nil {
			return err
		}
	}

	if spec.HuntDispatcher {
		// Hunt dispatcher manages client's hunt membership.
		err := sm.Start(hunt_dispatcher.StartHuntDispatcher)
		if err != nil {
			return err
		}
	}

	if spec.HuntManager {
		err := sm.Start(hunt_manager.StartHuntManager)
		if err != nil {
			return err
		}
	}

	// Interrogation service populates indexes etc for new
	// clients.
	if spec.Interrogation {
		err := sm.Start(interrogation.StartInterrogationService)
		if err != nil {
			return err
		}
	}

	// Runs server event queries. Should only run on one frontend.
	if spec.ServerMonitoring {
		err := sm.Start(server_monitoring.StartServerMonitoringService)
		if err != nil {
			return err
		}
	}

	// VFS service maintains the VFS GUI structures by parsing the
	// output of VFS artifacts collected.
	if spec.VfsService {
		err := sm.Start(vfs_service.StartVFSService)
		if err != nil {
			return err
		}
	}

	// Run any server artifacts the user asks for.
	if spec.ServerArtifacts {
		err := sm.Start(server_artifacts.StartServerArtifactService)
		if err != nil {
			return err
		}
	}

	return nil
}

func Reset() {
	// This function should not find any active services. Services
	// are responsible for unregistering themselves and holding
	// the service manager for the duration of their lifetime.

	journal, _ := services.GetJournal()
	if journal != nil {
		fmt.Printf("Journal not reset.\n")
	}

	if services.GetNotifier() != nil {
		fmt.Printf("Notifier not reset.\n")
	}

	if services.GetInventory() != nil {
		fmt.Printf("Inventory not reset.\n")
	}

	manager, _ := services.GetRepositoryManager()
	if manager != nil {
		fmt.Printf("Repository Manager not reset.\n")
	}

	launcher, _ := services.GetLauncher()
	if launcher != nil {
		fmt.Printf("Launcher not reset.\n")
	}

	if services.GetLabeler() != nil {
		fmt.Printf("Labeler not reset.\n")
	}

	if services.GetHuntDispatcher() != nil {
		fmt.Printf("HuntDispatcher not reset.\n")
	}

	services.RegisterJournal(nil)
	services.RegisterNotifier(nil)
	services.RegisterInventory(nil)
	services.RegisterRepositoryManager(nil)
	services.RegisterLauncher(nil)
	services.RegisterLabeler(nil)
	services.RegisterHuntDispatcher(nil)
	services.RegisterClientEventManager(nil)
}
