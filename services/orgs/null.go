package orgs

import (
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

type NullServiceContainer struct{}

func (self *NullServiceContainer) FrontendManager() (services.FrontendManager, error) {
	return nil, utils.NotImplementedError
}

func (self *NullServiceContainer) Journal() (services.JournalService, error) {
	return nil, utils.NotImplementedError
}
func (self *NullServiceContainer) ClientInfoManager() (services.ClientInfoManager, error) {
	return nil, utils.NotImplementedError
}
func (self *NullServiceContainer) Indexer() (services.Indexer, error) {
	return nil, utils.NotImplementedError
}
func (self *NullServiceContainer) BroadcastService() (services.BroadcastService, error) {
	return nil, utils.NotImplementedError
}
func (self *NullServiceContainer) Inventory() (services.Inventory, error) {
	return nil, utils.NotImplementedError
}
func (self *NullServiceContainer) VFSService() (services.VFSService, error) {
	return nil, utils.NotImplementedError
}
func (self *NullServiceContainer) Labeler() (services.Labeler, error) {
	return nil, utils.NotImplementedError
}
func (self *NullServiceContainer) RepositoryManager() (services.RepositoryManager, error) {
	return nil, utils.NotImplementedError
}
func (self *NullServiceContainer) HuntDispatcher() (services.IHuntDispatcher, error) {
	return nil, utils.NotImplementedError
}
func (self *NullServiceContainer) Launcher() (services.Launcher, error) {
	return nil, utils.NotImplementedError
}
func (self *NullServiceContainer) NotebookManager() (services.NotebookManager, error) {
	return nil, utils.NotImplementedError
}
func (self *NullServiceContainer) ClientEventManager() (services.ClientEventTable, error) {
	return nil, utils.NotImplementedError
}
func (self *NullServiceContainer) ServerEventManager() (services.ServerEventManager, error) {
	return nil, utils.NotImplementedError
}
func (self *NullServiceContainer) ServerArtifactRunner() (services.ServerArtifactRunner, error) {
	return nil, utils.NotImplementedError
}
func (self *NullServiceContainer) Notifier() (services.Notifier, error) {
	return nil, utils.NotImplementedError
}

func (self *NullServiceContainer) ACLManager() (services.ACLManager, error) {
	return nil, utils.NotImplementedError
}
func (self *NullServiceContainer) AuditManager() (services.AuditManager, error) {
	return nil, utils.NotImplementedError
}
func (self *NullServiceContainer) Scheduler() (services.Scheduler, error) {
	return nil, utils.NotImplementedError
}
func (self *NullServiceContainer) SecretsService() (services.SecretsService, error) {
	return nil, utils.NotImplementedError
}
func (self *NullServiceContainer) BackupService() (services.BackupService, error) {
	return nil, utils.NotImplementedError
}
func (self *NullServiceContainer) ExportManager() (services.ExportManager, error) {
	return nil, utils.NotImplementedError
}
func (self *NullServiceContainer) DocManager() (services.DocManager, error) {
	return nil, utils.NotImplementedError
}
func (self *NullServiceContainer) LSPServer() (services.LSPServer, error) {
	return nil, utils.NotImplementedError
}
