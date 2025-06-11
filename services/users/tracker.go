package users

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

func (self *UserManager) WriteProfile(ctx context.Context,
	scope vfilter.Scope, output_chan chan vfilter.Row) {
	storage, ok := self.storage.(*UserStorageManager)
	if ok {
		storage.WriteProfile(ctx, scope, output_chan)
	}
}

func (self *UserStorageManager) WriteProfile(ctx context.Context,
	scope vfilter.Scope, output_chan chan vfilter.Row) {

	var rows []*ordereddict.Dict

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return
	}

	self.mu.Lock()
	for key, cache := range self.cache {
		username := cache.user_record.Name

		for _, org := range org_manager.ListOrgs() {
			org_config_obj, err := org_manager.GetOrgConfig(org.Id)
			if err != nil {
				continue
			}

			policy, err := services.GetPolicy(org_config_obj, username)
			if err != nil {
				continue
			}

			rows = append(rows, ordereddict.NewDict().
				Set("Key", key).
				Set("Name", username).
				Set("LastRefresh", utils.GetTime().Now().
					Sub(cache.timestamp).Round(time.Second).String()).
				Set("OrgId", org.Id).
				Set("OrgName", org.Name).
				Set("Poilcy", policy))
		}
	}
	self.mu.Unlock()

	for _, r := range rows {
		select {
		case <-ctx.Done():
			return
		case output_chan <- r:
		}
	}

}
