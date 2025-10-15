/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

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
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	HuntNotFoundError = utils.Wrap(utils.NotFoundError, "Hunt no found")
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

type HuntSearchOptions int

const (
	AllHunts HuntSearchOptions = iota

	// Only visit non expired hunts
	OnlyRunningHunts
)

type FlowSearchOptions struct {
	result_sets.ResultSetOptions

	// Additional Options for efficient search.

	// BasicInformation includes only client id and flow id.
	BasicInformation bool
}

type IHuntDispatcher interface {
	// Applies the function on all the hunts. Functions may not
	// modify the hunt but will have read only access to the hunt
	// objects under lock.
	ApplyFuncOnHunts(ctx context.Context, options HuntSearchOptions,
		cb func(hunt *api_proto.Hunt) error) error

	// As an optimization callers may get the latest hunt's
	// timestamp. If the client's last hunt id is earlier than
	// this then we need to find out exactly which hunt is missing
	// from the client. Most of the time, clients will be up to
	// date on the latest hunt version and this will be a noop.
	GetLastTimestamp() uint64

	// Modify a hunt under lock. The hunt will be synchronized to all
	// frontends. Return HuntModificationAction to indicate if the
	// hunt was modified.
	// This function can only be called on the master node.
	ModifyHuntObject(ctx context.Context, hunt_id string,
		cb func(hunt *api_proto.Hunt) HuntModificationAction,
	) HuntModificationAction

	// Gets read only access to the hunt object.
	GetHunt(ctx context.Context,
		hunt_id string) (*api_proto.Hunt, bool)

	// Paged view into the flows in the hunt
	GetFlows(ctx context.Context, config_obj *config_proto.Config,
		options FlowSearchOptions, scope vfilter.Scope,
		hunt_id string, start int) (
		output chan *api_proto.FlowDetails, total_rows int64, err error)

	CreateHunt(ctx context.Context,
		config_obj *config_proto.Config,
		acl_manager vql_subsystem.ACLManager,
		hunt *api_proto.Hunt) (*api_proto.Hunt, error)

	// Deprecated - use GetHunts for paged access
	ListHunts(ctx context.Context,
		config_obj *config_proto.Config,
		in *api_proto.ListHuntsRequest) (*api_proto.ListHuntsResponse, error)

	// Paged access to hunts
	GetHunts(ctx context.Context,
		config_obj *config_proto.Config,
		options result_sets.ResultSetOptions,
		start_row, length int64) ([]*api_proto.Hunt, int64, error)

	// Send a mutation to a hunt object. Mutations allow the minions
	// to send updates to the master node which applies the change.
	MutateHunt(ctx context.Context,
		config_obj *config_proto.Config,
		mutation *api_proto.HuntMutation) error

	// Re-read the hunts from the data store. This happens
	// periodically and can also be triggered when a change is
	// written to the datastore (e.g. new hunt scheduled) to pick
	// up the latest hunts.
	Refresh(ctx context.Context, config_obj *config_proto.Config) error

	// Clean up and close the hunt dispatcher. Only used in tests.
	Close(ctx context.Context)

	// Get all known tags. Used for GUI suggestions
	GetTags(ctx context.Context) []string
}

func GetHuntDispatcher(config_obj *config_proto.Config) (IHuntDispatcher, error) {
	org_manager, err := GetOrgManager()
	if err != nil {
		return nil, err
	}

	return org_manager.Services(config_obj.OrgId).HuntDispatcher()
}
