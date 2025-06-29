package flows

import (
	"context"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
)

// Process ClientInfo messages. These are processed directly on the
// minions rathen than getting sent to event moniroting.
func (self *ClientFlowRunner) maybeProcessClientInfo(
	ctx context.Context, client_id string, response *actions_proto.VQLResponse) error {
	if response.Query == nil ||
		response.Query.Name != "Server.Internal.ClientInfo" {
		return nil
	}

	client_info := &services.ClientInfo{ClientInfo: &actions_proto.ClientInfo{}}
	err := json.Unmarshal([]byte(response.JSONLResponse), &client_info.ClientInfo)
	if err != nil {
		return err
	}

	// Update client info record if needed
	client_info_manager, err := services.GetClientInfoManager(self.config_obj)
	if err != nil {
		return err
	}

	err = client_info_manager.Modify(ctx, client_id,
		func(old_client_info *services.ClientInfo) (*services.ClientInfo, error) {
			if old_client_info == nil {
				return client_info, nil
			}

			dirty := false
			update := func(old, new *string) {
				if *old != *new {
					*old = *new
					dirty = true
				}
			}

			// Now merge the new record with the old
			old_client_info.ClientId = client_id
			update(&old_client_info.Hostname, &client_info.Hostname)
			update(&old_client_info.System, &client_info.System)
			update(&old_client_info.Release, &client_info.Release)
			update(&old_client_info.Architecture, &client_info.Architecture)
			update(&old_client_info.Fqdn, &client_info.Fqdn)
			update(&old_client_info.ClientName, &client_info.ClientName)
			update(&old_client_info.ClientVersion, &client_info.ClientVersion)
			update(&old_client_info.BuildUrl, &client_info.BuildUrl)
			update(&old_client_info.BuildTime, &client_info.BuildTime)

			// Nothing to do ignore the update.
			if !dirty {
				return nil, nil
			}

			// Update the index
			indexer, err := services.GetIndexer(self.config_obj)
			if err == nil {
				for _, term := range []string{
					"all",
					client_id,
					"host:" + client_info.Fqdn,
					"host:" + client_info.Hostname} {
					_ = indexer.SetIndex(client_id, term)
				}
			}

			return old_client_info, nil
		})
	if err != nil {
		return err
	}

	// Now update any labels baked into the client.
	if len(client_info.Labels) > 0 {
		labeler := services.GetLabeler(self.config_obj)

		for _, label := range client_info.Labels {
			err = labeler.SetClientLabel(ctx, self.config_obj, client_id, label)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
