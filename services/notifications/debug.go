package notifications

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

func (self *Notifier) WriteProfile(ctx context.Context,
	scope vfilter.Scope, output_chan chan vfilter.Row) {

	for _, client_id := range self.ListClients() {
		output_chan <- ordereddict.NewDict().
			Set("OrgId", utils.GetOrgId(self.config_obj)).
			Set("ClientId", client_id).
			Set("Hostname", services.GetHostname(ctx, self.config_obj, client_id))
	}
}
