package orgs

import (
	"errors"
	"sync"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/broadcast"
	"www.velocidex.com/golang/velociraptor/services/client_info"
	"www.velocidex.com/golang/velociraptor/services/client_monitoring"
	"www.velocidex.com/golang/velociraptor/services/frontend"
	"www.velocidex.com/golang/velociraptor/services/hunt_dispatcher"
	"www.velocidex.com/golang/velociraptor/services/hunt_manager"
	"www.velocidex.com/golang/velociraptor/services/indexing"
	"www.velocidex.com/golang/velociraptor/services/interrogation"
	"www.velocidex.com/golang/velociraptor/services/inventory"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/labels"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/services/notebook"
	"www.velocidex.com/golang/velociraptor/services/notifications"
	"www.velocidex.com/golang/velociraptor/services/repository"
	"www.velocidex.com/golang/velociraptor/services/sanity"
	"www.velocidex.com/golang/velociraptor/services/server_artifacts"
	"www.velocidex.com/golang/velociraptor/services/server_monitoring"
	"www.velocidex.com/golang/velociraptor/services/vfs_service"
)

type ServiceContainer struct {
	mu sync.Mutex

	frontend             services.FrontendManager
	journal              services.JournalService
	client_info_manager  services.ClientInfoManager
	indexer              services.Indexer
	broadcast            services.BroadcastService
	inventory            services.Inventory
	vfs_service          services.VFSService
	labeler              services.Labeler
	repository           services.RepositoryManager
	hunt_dispatcher      services.IHuntDispatcher
	launcher             services.Launcher
	notebook_manager     services.NotebookManager
	client_event_manager services.ClientEventTable
	server_event_manager services.ServerEventManager
	notifier             services.Notifier
}

func (self *ServiceContainer) FrontendManager() (services.FrontendManager, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.frontend == nil {
		return nil, errors.New("Frontend service not initialized")
	}

	return self.frontend, nil
}

func (self *ServiceContainer) Notifier() (services.Notifier, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.notifier == nil {
		return nil, errors.New("Notification service not initialized")
	}

	return self.notifier, nil
}

func (self *ServiceContainer) ServerEventManager() (services.ServerEventManager, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.server_event_manager == nil {
		return nil, errors.New("Server Monitoring Manager service not initialized")
	}

	return self.server_event_manager, nil
}

func (self *ServiceContainer) ClientEventManager() (services.ClientEventTable, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.client_event_manager == nil {
		return nil, errors.New("Client Monitoring Manager service not initialized")
	}

	return self.client_event_manager, nil
}

func (self *ServiceContainer) NotebookManager() (services.NotebookManager, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.notebook_manager == nil {
		return nil, errors.New("Notebook Manager service not initialized")
	}

	return self.notebook_manager, nil
}

func (self *ServiceContainer) Launcher() (services.Launcher, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.launcher == nil {
		return nil, errors.New("Launcher service not initialized")
	}

	return self.launcher, nil
}

func (self *ServiceContainer) HuntDispatcher() (services.IHuntDispatcher, error) {

	self.mu.Lock()
	defer self.mu.Unlock()

	if self.hunt_dispatcher == nil {
		return nil, errors.New("Hunt Dispatcher service not initialized")
	}

	return self.hunt_dispatcher, nil
}

func (self *ServiceContainer) Indexer() (services.Indexer, error) {

	self.mu.Lock()
	defer self.mu.Unlock()

	if self.indexer == nil {
		return nil, errors.New("Indexing service not initialized")
	}

	return self.indexer, nil
}

func (self *ServiceContainer) RepositoryManager() (services.RepositoryManager, error) {

	self.mu.Lock()
	defer self.mu.Unlock()

	if self.repository == nil {
		return nil, errors.New("Repository Manager service not initialized")
	}

	return self.repository, nil
}

func (self *ServiceContainer) VFSService() (services.VFSService, error) {

	self.mu.Lock()
	defer self.mu.Unlock()

	if self.vfs_service == nil {
		return nil, errors.New("VFS service not initialized")
	}

	return self.vfs_service, nil
}

func (self *ServiceContainer) Labeler() (services.Labeler, error) {

	self.mu.Lock()
	defer self.mu.Unlock()

	if self.labeler == nil {
		return nil, errors.New("Labeling service not initialized")
	}

	return self.labeler, nil
}

func (self *ServiceContainer) Journal() (services.JournalService, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.journal == nil {
		return nil, errors.New("Journal service not ready")
	}
	return self.journal, nil
}

func (self *ServiceContainer) ClientInfoManager() (services.ClientInfoManager, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.client_info_manager == nil {
		return nil, errors.New("Client Info Manager not ready")
	}
	return self.client_info_manager, nil
}

func (self *ServiceContainer) Inventory() (services.Inventory, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.inventory == nil {
		return nil, errors.New("Inventory Manager not ready")
	}
	return self.inventory, nil
}

func (self *ServiceContainer) BroadcastService() (services.BroadcastService, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.broadcast == nil {
		return nil, errors.New("Broadcast Service not ready")
	}
	return self.broadcast, nil
}

// Start all the services for the org and install it in the
// manager. This function is used both in the client and the server to
// start all the needed services.
func (self *OrgManager) startOrg(org_record *api_proto.OrgRecord) (err error) {
	org_config := self.makeNewConfigObj(org_record)
	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Info("Starting services for %v", services.GetOrgName(org_config))

	spec := org_config.Frontend.ServerServices
	if spec == nil {
		spec = services.AllServicesSpec()
	}

	self.mu.Lock()
	org_id := org_record.OrgId

	service_container := &ServiceContainer{}
	self.orgs[org_id] = &OrgContext{
		record:     org_record,
		config_obj: org_config,
		service:    service_container,
	}
	self.org_id_by_nonce[org_record.Nonce] = org_id
	self.mu.Unlock()

	if spec.FrontendServer && org_id == "" {
		f, err := frontend.NewFrontendService(
			self.ctx, self.wg, org_config)
		if err != nil {
			return err
		}
		service_container.mu.Lock()
		service_container.frontend = f
		service_container.mu.Unlock()
	}

	// Now start the services for this org. Services depend on other
	// services so they need to be accessible as soon as they are
	// ready.
	if spec.JournalService {
		j, err := journal.NewJournalService(
			self.ctx, self.wg, org_config)
		if err != nil {
			return err
		}
		service_container.mu.Lock()
		service_container.journal = j
		service_container.broadcast = broadcast.NewBroadcastService(org_config)
		service_container.mu.Unlock()
	}

	if spec.NotificationService {
		n, err := notifications.NewNotificationService(
			self.ctx, self.wg, org_config)
		if err != nil {
			return err
		}
		service_container.mu.Lock()
		service_container.notifier = n
		service_container.mu.Unlock()
	}

	if spec.TestRepositoryManager {
		repo_manager, err := repository.NewRepositoryManager(
			self.ctx, self.wg, org_config)
		if err != nil {
			return err
		}

		err = repository.LoadArtifactsFromConfig(repo_manager, org_config)
		if err != nil {
			return err
		}

		service_container.mu.Lock()
		service_container.repository = repo_manager
		service_container.mu.Unlock()
	}

	if spec.RepositoryManager {
		repo_manager, err := repository.NewRepositoryManager(
			self.ctx, self.wg, org_config)
		if err != nil {
			return err
		}

		err = repository.LoadArtifactsFromConfig(repo_manager, org_config)
		if err != nil {
			return err
		}

		// The Root org will contain all the built in artifacts
		if org_id == "" {
			// Assume the built in artifacts are OK so we dont need to
			// validate them at runtime.
			err = repository.LoadBuiltInArtifacts(self.ctx, org_config,
				repo_manager.(*repository.RepositoryManager), false /* validate */)
			if err != nil {
				return err
			}
		} else {
			root_org_config, _ := self.GetOrgConfig("")
			root_repo_manager, _ := self.Services("").RepositoryManager()
			root_repo, _ := root_repo_manager.GetGlobalRepository(root_org_config)
			repo_manager.SetParent(root_org_config, root_repo)

			global_repository, err := repo_manager.GetGlobalRepository(org_config)
			if err != nil {
				return err
			}

			_, err = repository.InitializeGlobalRepositoryFromFilestore(self.ctx, org_config, global_repository)
			if err != nil {
				return err
			}
		}

		service_container.mu.Lock()
		service_container.repository = repo_manager
		service_container.mu.Unlock()
	}

	if spec.InventoryService {
		i, err := inventory.NewInventoryService(
			self.ctx, self.wg, org_config)
		if err != nil {
			return err
		}

		service_container.mu.Lock()
		service_container.inventory = i
		service_container.mu.Unlock()
	}

	if spec.HuntDispatcher {
		hd, err := hunt_dispatcher.NewHuntDispatcher(
			self.ctx, self.wg, org_config)
		if err != nil {
			return err
		}

		service_container.mu.Lock()
		service_container.hunt_dispatcher = hd
		service_container.mu.Unlock()
	}

	if spec.HuntManager {
		err = hunt_manager.NewHuntManager(
			self.ctx, self.wg, org_config)
		if err != nil {
			return err
		}
	}

	if spec.Interrogation {
		err = interrogation.NewInterrogationService(
			self.ctx, self.wg, org_config)

		if err != nil {
			return err
		}
	}

	if spec.ClientInfo {
		c := client_info.NewClientInfoManager(org_config)
		err = c.Start(self.ctx, org_config, self.wg)
		if err != nil {
			return err
		}

		service_container.mu.Lock()
		service_container.client_info_manager = c
		service_container.mu.Unlock()
	}

	if spec.IndexServer {
		inv, err := indexing.NewIndexingService(self.ctx, self.wg, org_config)
		if err != nil {
			return err
		}

		service_container.mu.Lock()
		service_container.indexer = inv
		service_container.mu.Unlock()
	}

	if spec.VfsService {
		vfs, err := vfs_service.NewVFSService(
			self.ctx, self.wg, org_config)
		if err != nil {
			return err
		}

		service_container.mu.Lock()
		service_container.vfs_service = vfs
		service_container.mu.Unlock()
	}

	if spec.Label {
		l, err := labels.NewLabelerService(
			self.ctx, self.wg, org_config)
		if err != nil {
			return err
		}

		service_container.mu.Lock()
		service_container.labeler = l
		service_container.mu.Unlock()
	}

	if spec.Launcher {
		launch, err := launcher.NewLauncherService(
			self.ctx, self.wg, org_config)
		if err != nil {
			return err
		}

		service_container.mu.Lock()
		service_container.launcher = launch
		service_container.mu.Unlock()
	}

	if spec.NotebookService {
		nb, err := notebook.NewNotebookManagerService(self.ctx, self.wg, org_config)
		if err != nil {
			return err
		}

		service_container.mu.Lock()
		service_container.notebook_manager = nb
		service_container.mu.Unlock()
	}

	if spec.SanityChecker {
		err = sanity.NewSanityCheckService(self.ctx, self.wg, org_config)
		if err != nil {
			return err
		}
	}

	if spec.ServerArtifacts {
		err = server_artifacts.NewServerArtifactService(self.ctx, self.wg, org_config)
		if err != nil {
			return err
		}
	}

	if spec.ClientMonitoring {
		client_event_manager, err := client_monitoring.NewClientMonitoringService(self.ctx, self.wg, org_config)
		if err != nil {
			return err
		}

		service_container.mu.Lock()
		service_container.client_event_manager = client_event_manager
		service_container.mu.Unlock()
	}

	if spec.MonitoringService {
		server_event_manager, err := server_monitoring.NewServerMonitoringService(self.ctx, self.wg, org_config)
		if err != nil {
			return err
		}

		service_container.mu.Lock()
		service_container.server_event_manager = server_event_manager
		service_container.mu.Unlock()
	}
	return err
}

func (self *OrgManager) Services(org_id string) services.ServiceContainer {
	self.mu.Lock()
	defer self.mu.Unlock()

	service_container, pres := self.orgs[org_id]
	if !pres {
		return &ServiceContainer{}
	}
	return service_container.service
}
