package client_info

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self *ClientInfoManager) DeleteClient(
	ctx context.Context,
	client_id, principal string,
	progress chan services.DeleteFlowResponse, really_do_it bool) error {

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	file_store_factory := file_store.GetFileStore(self.config_obj)
	client_path_manager := paths.NewClientPathManager(client_id)

	if really_do_it {
		err := self.clearIndexer(ctx, client_id, principal)
		if err != nil && progress != nil {
			progress <- services.DeleteFlowResponse{
				Error: fmt.Sprintf("client_delete: clearIndexer: %v", err),
			}
		}
	}

	// Indiscriminately delete all the client's datastore files.
	_ = datastore.Walk(self.config_obj, db, client_path_manager.Path(),
		datastore.WalkWithoutDirectories,
		func(filename api.DSPathSpec) error {
			item := services.DeleteFlowResponse{
				Type: "Datastore",
				Data: ordereddict.NewDict().
					Set("vfs_path", filename.AsClientPath()),
			}

			if really_do_it {
				err := db.DeleteSubject(self.config_obj, filename)
				if err != nil && errors.Is(err, os.ErrNotExist) {
					item.Error = fmt.Sprintf("client_delete: while deleting %v: %s",
						filename, err)
				}
			}

			if progress != nil {
				select {
				case <-ctx.Done():
					return nil

				case progress <- item:
				}
			}

			return nil
		})

	// Delete the filestore files.
	err = api.Walk(file_store_factory,
		client_path_manager.Path().AsFilestorePath(),
		func(filename api.FSPathSpec, info os.FileInfo) error {
			item := services.DeleteFlowResponse{
				Type: "Filestore",
				Data: ordereddict.NewDict().
					Set("vfs_path", filename.AsClientPath()),
			}

			if really_do_it {
				err := file_store_factory.Delete(filename)
				if err != nil {
					item.Error = fmt.Sprintf("client_delete: while deleting %v: %s",
						filename, err)
				}
			}

			if progress != nil {
				select {
				case <-ctx.Done():
					return nil

				case progress <- item:
				}
			}

			return nil
		})
	if err != nil {
		return err
	}

	// Remove the empty directories
	err = datastore.Walk(self.config_obj, db, client_path_manager.Path(),
		datastore.WalkWithDirectories,
		func(filename api.DSPathSpec) error {
			err := db.DeleteSubject(self.config_obj, filename)
			if err != nil && progress != nil {
				progress <- services.DeleteFlowResponse{
					Type: "DeleteDirectory",
					Data: ordereddict.NewDict().
						Set("vfs_path", filename.AsClientPath()),
					Error: fmt.Sprintf("client_delete: Removing directory %v: %v",
						filename.AsClientPath(), err),
				}
			}
			return nil
		})
	if err != nil {
		return err
	}

	// Delete the actual client record.
	if really_do_it {
		err = self.reallyDeleteClient(ctx, client_id, principal)
		if err != nil && progress != nil {
			progress <- services.DeleteFlowResponse{
				Error: fmt.Sprintf("client_delete: reallyDeleteClient %s", err),
			}
		}

		// Finally remove the containing directory
		err = db.DeleteSubject(
			self.config_obj,
			paths.NewClientPathManager(client_id).Path().SetDir())
		if err != nil && !errors.Is(err, os.ErrNotExist) && progress != nil {
			progress <- services.DeleteFlowResponse{
				Error: fmt.Sprintf("client_delete: reallyDeleteClient %s", err),
			}
		}
	}

	// Notify the client to force it to disconnect in case
	// it is already up.
	notifier, err := services.GetNotifier(self.config_obj)
	if err == nil {
		err = notifier.NotifyListener(ctx,
			self.config_obj, client_id, "DeleteClient")
		if err != nil && progress != nil {
			progress <- services.DeleteFlowResponse{
				Error: fmt.Sprintf("client_delete: reallyDeleteClient %s", err),
			}
		}
	}

	return nil
}

func (self *ClientInfoManager) clearIndexer(ctx context.Context,
	client_id string, principal string) error {

	indexer, err := services.GetIndexer(self.config_obj)
	if err != nil {
		return err
	}

	client_info, err := indexer.FastGetApiClient(ctx,
		self.config_obj, client_id)
	if err != nil {
		return err
	}

	// Remove any labels
	labeler := services.GetLabeler(self.config_obj)
	for _, label := range labeler.GetClientLabels(ctx, self.config_obj,
		client_id) {
		err := labeler.RemoveClientLabel(ctx, self.config_obj,
			client_id, label)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	// Sync up with the indexes created by the interrogation service.
	keywords := []string{"all", client_info.ClientId}
	if client_info.OsInfo != nil && client_info.OsInfo.Fqdn != "" {
		keywords = append(keywords, "host:"+client_info.OsInfo.Hostname)
		keywords = append(keywords, "host:"+client_info.OsInfo.Fqdn)
	}
	for _, keyword := range keywords {
		err = indexer.UnsetIndex(client_id, keyword)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	return nil
}

func (self *ClientInfoManager) reallyDeleteClient(ctx context.Context,
	client_id string, principal string) error {

	defer self.Remove(ctx, client_id)

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	client_path_manager := paths.NewClientPathManager(client_id)
	err = db.DeleteSubject(self.config_obj, client_path_manager.Path())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	journal, err := services.GetJournal(self.config_obj)
	if err != nil {
		return err
	}

	err = services.LogAudit(ctx,
		self.config_obj, principal, "client_delete",
		ordereddict.NewDict().
			Set("client_id", client_id).
			Set("org_id", self.config_obj.OrgId))
	if err != nil {
		return err
	}

	err = journal.PushRowsToArtifact(ctx, self.config_obj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("ClientId", client_id).
			Set("OrgId", self.config_obj.OrgId).
			Set("Principal", principal)},
		"Server.Internal.ClientDelete", "server", "")

	if err != nil {
		return err
	}

	// Send an event that the client was deleted to the root org as
	// well. The Frontend is not org aware and needs to be informed to
	// client deletion events.
	if utils.IsRootOrg(self.config_obj.OrgId) {
		return nil
	}

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return err
	}

	root_config_obj, err := org_manager.GetOrgConfig("root")
	if err != nil {
		return err
	}

	journal, err = services.GetJournal(root_config_obj)
	if err != nil {
		return err
	}

	return journal.PushRowsToArtifact(ctx, root_config_obj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("ClientId", client_id).
			Set("OrgId", self.config_obj.OrgId).
			Set("Principal", principal)},
		"Server.Internal.ClientDelete", "server", "")
}
