package orgs

import (
	"errors"
	"os"
	"time"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

func RemoveOrgFromUsers(org_id string) error {
	// Remove the org from all the users.
	user_manager := services.GetUserManager()
	users, err := user_manager.ListUsers()
	if err != nil {
		return err
	}

	for _, u := range users {
		record, err := user_manager.GetUserWithHashes(u.Name)
		if err == nil {
			new_orgs := []*api_proto.Org{}
			for _, org := range record.Orgs {
				if org.Id != org_id {
					new_orgs = append(new_orgs, org)
				}
			}
			if len(new_orgs) != len(record.Orgs) {
				record.Orgs = new_orgs
				_ = user_manager.SetUser(record)
			}
		}
	}

	return nil
}

func (self OrgManager) DeleteOrg(org_id string) error {
	if utils.IsRootOrg(org_id) {
		return errors.New("Can not remove root org.")
	}

	err := RemoveOrgFromUsers(org_id)
	if err != nil {
		return err
	}

	org_path_manager := paths.NewOrgPathManager(org_id)
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return nil
	}

	err = db.DeleteSubject(self.config_obj, org_path_manager.Path())
	if err != nil {
		return nil
	}

	// Remove the org from the manager and cancel all its services.
	self.mu.Lock()
	org_context, pres := self.orgs[org_id]
	if pres {
		org_context.cancel()
		delete(self.orgs, org_id)
		delete(self.org_id_by_nonce, org_id)
	}
	self.mu.Unlock()

	if org_context == nil {
		return nil
	}

	// Wait a bit for the services to shut down so we can remove files
	// safely.
	go func() {
		time.Sleep(10 * time.Second)

		datastore.Walk(self.config_obj, db, org_path_manager.OrgDirectories(),
			datastore.WalkWithoutDirectories,
			func(path api.DSPathSpec) error {
				_ = db.DeleteSubject(self.config_obj, path)
				return nil
			})

		file_store_factory := file_store.GetFileStore(self.config_obj)
		api.Walk(file_store_factory,
			org_path_manager.OrgDirectories().AsFilestorePath(),
			func(path api.FSPathSpec, info os.FileInfo) error {
				file_store_factory.Delete(path)
				return nil
			})
	}()

	return nil
}
