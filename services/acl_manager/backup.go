package acl_manager

import (
	"context"
	"fmt"
	"sync"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type ACLBackupProvider struct {
	config_obj *config_proto.Config
	manager    *ACLManager
}

func (self ACLBackupProvider) ProviderName() string {
	return "ACLBackupProvider"
}

func (self ACLBackupProvider) Name() []string {
	return []string{"acls.json"}
}

func (self ACLBackupProvider) BackupResults(
	ctx context.Context, wg *sync.WaitGroup,
	container services.BackupContainerWriter) (<-chan vfilter.Row, error) {

	users_manager := services.GetUserManager()
	user_list, err := users_manager.ListUsers(ctx,
		// Superuser privileges ....
		utils.GetSuperuserName(self.config_obj),

		// Only export current org.
		[]string{utils.GetOrgId(self.config_obj)})
	if err != nil {
		return nil, err
	}

	output := make(chan vfilter.Row)

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(output)

		for _, user := range user_list {
			policy, err := self.manager.GetPolicy(
				self.config_obj, user.Name)
			if err == nil {
				select {
				case <-ctx.Done():
					return

				case output <- ordereddict.NewDict().
					Set("Principal", user).
					Set("Policy", policy):
				}
			}
		}
	}()

	return output, nil
}

// We do not want to automatically restore ACL permissions from backup
// because this may represent a security compromise but we want to
// allow users to see the ACL permissions that were backed up.
func (self ACLBackupProvider) Restore(ctx context.Context,
	container services.BackupContainerReader,
	in <-chan vfilter.Row) (stat services.BackupStat, err error) {

	count := 0
	defer func() {
		stat.Message = fmt.Sprintf("ACL backups do not automatically restore. There are %v ACL records which may be restored manually.", count)
	}()

	for _ = range in {
		count++
	}

	return stat, nil
}
