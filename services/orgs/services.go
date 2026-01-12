package orgs

import (
	"context"
	"errors"
	"sync"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/acl_manager"
	"www.velocidex.com/golang/velociraptor/services/audit_manager"
	"www.velocidex.com/golang/velociraptor/services/backup"
	"www.velocidex.com/golang/velociraptor/services/broadcast"
	"www.velocidex.com/golang/velociraptor/services/client_info"
	"www.velocidex.com/golang/velociraptor/services/client_monitoring"
	"www.velocidex.com/golang/velociraptor/services/ddclient"
	"www.velocidex.com/golang/velociraptor/services/docs"
	"www.velocidex.com/golang/velociraptor/services/exports"
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
	"www.velocidex.com/golang/velociraptor/services/scheduler"
	"www.velocidex.com/golang/velociraptor/services/secrets"
	"www.velocidex.com/golang/velociraptor/services/server_artifacts"
	"www.velocidex.com/golang/velociraptor/services/server_monitoring"
	"www.velocidex.com/golang/velociraptor/services/users"
	"www.velocidex.com/golang/velociraptor/services/vfs_service"
	"www.velocidex.com/golang/velociraptor/utils"
)

type ServiceContainer struct {
	mu sync.Mutex

	frontend                services.FrontendManager
	journal                 services.JournalService
	client_info_manager     services.ClientInfoManager
	indexer                 services.Indexer
	broadcast               services.BroadcastService
	inventory               services.Inventory
	vfs_service             services.VFSService
	labeler                 services.Labeler
	repository              services.RepositoryManager
	hunt_dispatcher         services.IHuntDispatcher
	launcher                services.Launcher
	notebook_manager        services.NotebookManager
	scheduler               services.Scheduler
	client_event_manager    services.ClientEventTable
	server_event_manager    services.ServerEventManager
	server_artifact_manager services.ServerArtifactRunner
	notifier                services.Notifier
	acl_manager             services.ACLManager
	secrets                 services.SecretsService
	backups                 services.BackupService
	export_manager          services.ExportManager
	doc_manager             services.DocManager
}

func (self *ServiceContainer) MockFrontendManager(svc services.FrontendManager) {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.frontend = svc
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

func (self *ServiceContainer) ServerArtifactRunner() (services.ServerArtifactRunner, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.server_artifact_manager == nil {
		return nil, errors.New("Server Artifact Runner service not initialized")
	}

	return self.server_artifact_manager, nil
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

func (self *ServiceContainer) SecretsService() (services.SecretsService, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.secrets == nil {
		return nil, errors.New("Secrets service not initialized")
	}

	return self.secrets, nil
}

func (self *ServiceContainer) AuditManager() (services.AuditManager, error) {
	return &audit_manager.AuditManager{}, nil
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

func (self *ServiceContainer) Scheduler() (services.Scheduler, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.scheduler == nil {
		return nil, errors.New("Scheduler service not initialized")
	}

	return self.scheduler, nil
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

func (self *ServiceContainer) DocManager() (services.DocManager, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.doc_manager == nil {
		return nil, errors.New("Doc Manager service not ready")
	}
	return self.doc_manager, nil
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

func (self *ServiceContainer) ExportManager() (services.ExportManager, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.export_manager == nil {
		return nil, errors.New("ExportManager Service not ready")
	}
	return self.export_manager, nil
}

func (self *ServiceContainer) BackupService() (services.BackupService, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.backups == nil {
		return nil, errors.New("Backup Service not ready")
	}
	return self.backups, nil
}

func (self *ServiceContainer) ACLManager() (services.ACLManager, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.acl_manager == nil {
		return nil, errors.New("ACLManager Service not ready")
	}
	return self.acl_manager, nil
}

// Start all the services for the org and install it in the
// manager. This function is used both in the client and the server to
// start all the needed services.
func (self *OrgManager) startOrg(org_record *api_proto.OrgRecord) (err error) {

	org_record.Id = utils.NormalizedOrgId(org_record.Id)

	org_config := self.makeNewConfigObj(org_record)
	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Info("Starting services for %v", services.GetOrgName(org_config))

	orgStartCounter.Inc()

	org_ctx := &OrgContext{
		record:     org_record,
		config_obj: org_config,
		service:    &ServiceContainer{},
		sm:         services.NewServiceManager(self.ctx, org_config),
	}

	// Make our parent waits for all the services to properly
	// exit. Each org service can be stopped independently but we can
	// not exit the org manager until they all shut down properly.
	self.parent_wg.Add(1)
	go func() {
		defer self.parent_wg.Done()

		<-org_ctx.sm.Ctx.Done()
		org_ctx.sm.Wg.Wait()
	}()

	self.mu.Lock()
	self.orgs[org_record.Id] = org_ctx
	self.org_id_by_nonce[org_record.Nonce] = org_record.Id
	self.mu.Unlock()

	return self.startOrgFromContext(org_ctx)
}

func (self *OrgManager) startRootOrgServices(
	ctx context.Context,
	wg *sync.WaitGroup,
	spec *config_proto.ServerServicesConfig,
	org_config *config_proto.Config,
	service_container *ServiceContainer) (err error) {

	// The MemcacheFileDataStore service
	err = datastore.StartMemcacheFileService(ctx, wg, org_config)
	if err != nil {
		return err
	}

	if spec.ReplicationService {
		j, err := journal.NewReplicationService(ctx, wg, org_config)
		if err != nil {
			return err
		}
		service_container.mu.Lock()
		service_container.journal = j
		service_container.broadcast = broadcast.NewBroadcastService(org_config)
		service_container.mu.Unlock()
	}

	// The user manager is global across all orgs.
	if spec.UserManager {
		err := users.StartUserManager(ctx, wg, org_config)
		if err != nil {
			return err
		}
	}

	err = ddclient.StartDynDNSService(
		ctx, wg, org_config)
	if err != nil {
		return err
	}

	err = datastore.StartDatastore(
		ctx, wg, org_config)
	if err != nil {
		return err
	}

	if spec.SchedulerService {
		err := scheduler.StartSchedulerService(ctx, wg, org_config)
		if err != nil {
			return err
		}
	}

	return nil
}

func (self *OrgManager) startOrgFromContext(org_ctx *OrgContext) (err error) {
	org_id := org_ctx.record.Id
	org_config := org_ctx.config_obj
	ctx := org_ctx.sm.Ctx
	wg := org_ctx.sm.Wg
	service_container := org_ctx.service.(*ServiceContainer)

	// If there is no frontend defined we are running as a client.
	spec := services.ClientServicesSpec()
	if org_config.Services != nil {
		spec = org_config.Services
	}

	if spec.BackupService {
		service_container.mu.Lock()
		service_container.backups = backup.NewBackupService(ctx, wg, org_config)
		service_container.mu.Unlock()
	}

	if spec.FrontendServer {
		f, err := frontend.NewFrontendService(ctx, wg, org_config)
		if err != nil {
			return err
		}
		service_container.mu.Lock()
		service_container.frontend = f
		service_container.mu.Unlock()
	}

	if spec.JournalService {
		j, err := journal.NewJournalService(ctx, wg, org_config)
		if err != nil {
			return err
		}
		service_container.mu.Lock()
		service_container.journal = j
		service_container.broadcast = broadcast.NewBroadcastService(org_config)
		service_container.mu.Unlock()
	}

	// Now start service on the root org
	if utils.IsRootOrg(org_id) {
		err := self.startRootOrgServices(ctx, wg, spec, org_config, service_container)
		if err != nil {
			return err
		}
	}

	// ACL manager exist for each org
	if spec.UserManager {
		m, err := acl_manager.NewACLManager(ctx, wg, org_config)
		if err != nil {
			return err
		}

		service_container.mu.Lock()
		service_container.acl_manager = m
		service_container.mu.Unlock()

		s, err := secrets.NewSecretsService(ctx, wg, org_config)
		if err != nil {
			return err
		}

		service_container.mu.Lock()
		service_container.secrets = s
		service_container.mu.Unlock()
	}

	// Now start the services for this org. Services depend on other
	// services so they need to be accessible as soon as they are
	// ready.
	if spec.NotificationService {
		n, err := notifications.NewNotificationService(ctx, wg, org_config)
		if err != nil {
			return err
		}
		service_container.mu.Lock()
		service_container.notifier = n
		service_container.mu.Unlock()
	}

	if spec.TestRepositoryManager {
		repo_manager, err := repository.NewRepositoryManager(
			ctx, wg, org_config)
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

	if spec.Launcher {
		launch, err := launcher.NewLauncherService(
			ctx, wg, org_config)
		if err != nil {
			return err
		}

		service_container.mu.Lock()
		service_container.launcher = launch
		service_container.mu.Unlock()
	}

	// Inventory service needs to start before we import built in
	// artifacts so they can add their tool dependencies.
	if spec.InventoryService {
		i, err := inventory.NewInventoryService(ctx, wg, org_config)
		if err != nil {
			return err
		}

		service_container.mu.Lock()
		service_container.inventory = i
		service_container.mu.Unlock()
	}

	if spec.RepositoryManager {
		repo_manager, err := repository.NewRepositoryManager(
			ctx, wg, org_config)
		if err != nil {
			return err
		}

		// The Root org will contain all the built in artifacts
		if utils.IsRootOrg(org_id) {
			// These artifacts are compiled in.
			err = repository.LoadBuiltInArtifacts(ctx, org_config,
				repo_manager.(*repository.RepositoryManager))
			if err != nil {
				return err
			}

			global_repository, err := repo_manager.GetGlobalRepository(org_config)
			if err != nil {
				return err
			}

			_, err = repository.InitializeGlobalRepositoryFromFilestore(
				ctx, org_config, global_repository)
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

			_, err = repository.InitializeGlobalRepositoryFromFilestore(ctx, org_config, global_repository)
			if err != nil {
				return err
			}
		}

		// Load config artifacts last so they can override all the
		// other artifacts.
		err = repository.LoadArtifactsFromConfig(repo_manager, org_config)
		if err != nil {
			return err
		}

		dm, err := docs.NewDocManager(ctx, wg, org_config)
		if err != nil {
			return err
		}

		service_container.mu.Lock()
		service_container.repository = repo_manager
		service_container.doc_manager = dm
		service_container.mu.Unlock()
	}

	if spec.HuntDispatcher {
		hd, err := hunt_dispatcher.NewHuntDispatcher(
			ctx, wg, org_config)
		if err != nil {
			return err
		}

		export_manager, err := exports.NewExportManager(
			ctx, wg, org_config)
		if err != nil {
			return err
		}

		service_container.mu.Lock()
		service_container.hunt_dispatcher = hd
		service_container.export_manager = export_manager
		service_container.mu.Unlock()
	}

	if spec.HuntManager {
		err = hunt_manager.NewHuntManager(
			ctx, wg, org_config)
		if err != nil {
			return err
		}
	}

	if spec.Interrogation {
		err = interrogation.NewInterrogationService(
			ctx, wg, org_config)

		if err != nil {
			return err
		}
	}

	if spec.ClientInfo {
		c, err := client_info.NewClientInfoManager(ctx, wg, org_config)
		if err != nil {
			return err
		}
		err = c.Start(ctx, org_config, wg)
		if err != nil {
			return err
		}

		service_container.mu.Lock()
		service_container.client_info_manager = c
		service_container.mu.Unlock()
	}

	if spec.IndexServer {
		inv, err := indexing.NewIndexingService(ctx, wg, org_config)
		if err != nil {
			return err
		}

		service_container.mu.Lock()
		service_container.indexer = inv
		service_container.mu.Unlock()
	}

	if spec.VfsService {
		vfs, err := vfs_service.NewVFSService(
			ctx, wg, org_config)
		if err != nil {
			return err
		}

		service_container.mu.Lock()
		service_container.vfs_service = vfs
		service_container.mu.Unlock()
	}

	if spec.Label {
		l, err := labels.NewLabelerService(
			ctx, wg, org_config)
		if err != nil {
			return err
		}

		service_container.mu.Lock()
		service_container.labeler = l
		service_container.mu.Unlock()
	}

	if spec.NotebookService {
		nb, err := notebook.NewNotebookManagerService(ctx, wg, org_config)
		if err != nil {
			return err
		}

		service_container.mu.Lock()
		service_container.notebook_manager = nb
		service_container.mu.Unlock()
	}

	if spec.ServerArtifacts {
		sm, err := server_artifacts.NewServerArtifactService(ctx, wg, org_config)
		if err != nil {
			return err
		}
		service_container.mu.Lock()
		service_container.server_artifact_manager = sm
		service_container.mu.Unlock()
	}

	if spec.ClientMonitoring {
		client_event_manager, err := client_monitoring.NewClientMonitoringService(ctx, wg, org_config)
		if err != nil {
			return err
		}

		service_container.mu.Lock()
		service_container.client_event_manager = client_event_manager
		service_container.mu.Unlock()
	}

	if spec.MonitoringService {
		server_event_manager, err := server_monitoring.NewServerMonitoringService(ctx, wg, org_config)
		if err != nil {
			return err
		}

		service_container.mu.Lock()
		service_container.server_event_manager = server_event_manager
		service_container.mu.Unlock()
	}

	// Must be run after all the other services are up
	if spec.SanityChecker {
		err = sanity.NewSanityCheckService(ctx, wg, org_config)
		if err != nil {
			return err
		}
	}

	return maybeFlushFilesOnClose(ctx, wg, org_config)
}

// Flush the datastore if possible when the org is closed to ensure
// all its data is flushed to disk. Some data stores delay writes so
// we need to make sure all the datastore files hit the disk before we
// close the org - for example if we delete the org subsequently we
// need to ensure no file writes are still in flight while we delete.
func maybeFlushFilesOnClose(
	ctx context.Context,
	wg *sync.WaitGroup,
	org_config *config_proto.Config) error {
	if org_config.Datastore == nil {
		return nil
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()

		logger := logging.GetLogger(org_config, &logging.FrontendComponent)
		err := file_store.FlushFilestore(org_config)
		if err != nil {
			logger.Error("<red>maybeFlushFilesOnClose FlushFilestore</> %v", err)
		}
		err = datastore.FlushDatastore(org_config)
		if err != nil {
			logger.Error("<red>maybeFlushFilesOnClose FlushDatastore</> %v", err)
		}
	}()

	return nil
}

func (self *OrgManager) Services(org_id string) services.ServiceContainer {
	self.mu.Lock()
	defer self.mu.Unlock()

	org_id = utils.NormalizedOrgId(org_id)

	service_container, pres := self.orgs[org_id]
	if !pres {
		return &ServiceContainer{}
	}
	return service_container.service
}

type Flusher interface {
	Flush()
}
