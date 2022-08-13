/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2022 Rapid7 Inc.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package services

// The hunt dispatcher service maintains in-memory information about
// active hunts. This is a time critical service designed to avoid
// locking as much as possible, since it is in the critical path of
// client requests.

import (
	"context"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

// How was the hunt modified and what should be done about it?
type HuntModificationAction int

const (
	// No modifications made - just ignore the changes.
	HuntUnmodified HuntModificationAction = iota

	// Changes should be propagated to all other hunt dispatchers on
	// all frontends.
	HuntPropagateChanges

	// Hunt was changed - all other hunt dispatchers should trigger
	// participation for this hunt. Used when a hunt is started.
	HuntTriggerParticipation

	// Just write to data store but do not propagate (good for very
	// frequent changes).
	HuntFlushToDatastore

	// Arrange for the change to be eventually written to the data
	// store but not right away. Useful for very low priority events.
	HuntFlushToDatastoreAsync
)

type IHuntDispatcher interface {
	// Applies the function on all the hunts. Functions may not
	// modify the hunt but will have read only access to the hunt
	// objects under lock.
	ApplyFuncOnHunts(cb func(hunt *api_proto.Hunt) error) error

	// As an optimization callers may get the latest hunt's
	// timestamp. If the client's last hunt id is earlier than
	// this then we need to find out exactly which hunt is missing
	// from the client. Most of the time, clients will be up to
	// date on the latest hunt version and this will be a noop.
	GetLastTimestamp() uint64

	// Modify a hunt under lock. The hunt will be synchronized to
	// all frontends. Return true to indicate the hunt was modified.
	ModifyHuntObject(hunt_id string,
		cb func(hunt *api_proto.Hunt) HuntModificationAction,
	) HuntModificationAction

	ModifyHunt(
		ctx context.Context,
		config_obj *config_proto.Config,
		hunt_modification *api_proto.Hunt,
		user string) error

	// Gets read only access to the hunt object.
	GetHunt(hunt_id string) (*api_proto.Hunt, bool)

	GetFlows(
		ctx context.Context,
		config_obj *config_proto.Config,
		scope vfilter.Scope,
		hunt_id string, start int) chan *api_proto.FlowDetails

	CreateHunt(ctx context.Context,
		config_obj *config_proto.Config,
		acl_manager vql_subsystem.ACLManager,
		hunt *api_proto.Hunt) (string, error)

	ListHunts(ctx context.Context,
		config_obj *config_proto.Config,
		in *api_proto.ListHuntsRequest) (*api_proto.ListHuntsResponse, error)

	// Send a mutation to a hunt object.
	MutateHunt(config_obj *config_proto.Config,
		mutation *api_proto.HuntMutation) error

	// Re-read the hunts from the data store. This happens
	// periodically and can also be triggered when a change is
	// written to the datastore (e.g. new hunt scheduled) to pick
	// up the latest hunts.
	Refresh(config_obj *config_proto.Config) error

	// Clean up and close the hunt dispatcher. Only used in tests.
	Close(config_obj *config_proto.Config)
}

func GetHuntDispatcher(config_obj *config_proto.Config) (IHuntDispatcher, error) {
	org_manager, err := GetOrgManager()
	if err != nil {
		return nil, err
	}

	return org_manager.Services(config_obj.OrgId).HuntDispatcher()
}
