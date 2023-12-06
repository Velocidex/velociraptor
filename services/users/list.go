package users

import (
	"context"
	"sort"

	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

// List all the users visible to this principal.
// - If the principal is an ORG_ADMIN they can see all users
// - If the user is SERVER_ADMIN they will see the full user records.
// - Otherwise list all users belonging to the orgs in which the user
//   has at least read access. Only user names will be shown.
func (self *UserManager) ListUsers(
	ctx context.Context,
	principal string, orgs []string) ([]*api_proto.VelociraptorUser, error) {

	// ORG_ADMINs can see everything
	principal_is_org_admin, _ := services.CheckAccess(self.config_obj,
		principal, acls.ORG_ADMIN)

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return nil, err
	}

	users, err := self.storage.ListAllUsers(ctx)
	if err != nil {
		return nil, err
	}

	// Filter the users according to the access
	result := make([]*api_proto.VelociraptorUser, 0, len(users))
	type org_info struct {
		// If the principal is only a user in this org they can only
		// see partial records.
		allowed bool

		// If the principal is an admin in this org they can see the
		// full record.
		allowed_full bool

		org_config_obj *config_proto.Config
		org_name       string
	}

	// A plan of which orgs will be visible to the principal.
	plan := make(map[string]*org_info)
	for _, org := range org_manager.ListOrgs() {
		// Caller is only interested in these orgs.
		if len(orgs) > 0 && !utils.OrgIdInList(org.Id, orgs) {
			continue
		}

		info := &org_info{
			org_name: org.Name,
		}

		info.org_config_obj, err = org_manager.GetOrgConfig(org.Id)
		if err != nil {
			continue
		}

		if principal_is_org_admin {
			info.allowed_full = true
			plan[org.Id] = info
			continue
		}

		// Server admin can see the full record.
		allowed, _ := services.CheckAccess(info.org_config_obj,
			principal, acls.SERVER_ADMIN)
		if allowed {
			info.allowed_full = true
			plan[org.Id] = info
			continue
		}

		// A user in that org can see a partial record.
		allowed, _ = services.CheckAccess(info.org_config_obj,
			principal, acls.READ_RESULTS)
		if allowed {
			info.allowed = true
			plan[org.Id] = info
			continue
		}
	}

	// Filtering the user list according to the following criteria:
	// The principal can see that org
	// The user has at least read permission in the org.
	for _, user := range users {
		user.Orgs = nil

		// This is the record we will return.
		user_record := &api_proto.VelociraptorUser{
			Name: user.Name,
		}

		for org_id, org_info := range plan {
			// Is the user in this org?
			user_in_org, err := services.CheckAccess(org_info.org_config_obj,
				user.Name, acls.READ_RESULTS)
			if err != nil || !user_in_org {
				continue
			}

			user_record.Orgs = append(user_record.Orgs, &api_proto.OrgRecord{
				Id:   utils.NormalizedOrgId(org_id),
				Name: org_info.org_name,
			})
		}

		// No orgs are visible - hide the user
		if len(user_record.Orgs) == 0 {
			continue
		}

		// Sort orgs for stable output
		sort.Slice(user_record.Orgs, func(i, j int) bool {
			return user_record.Orgs[i].Id < user_record.Orgs[j].Id
		})

		result = append(result, user_record)
	}

	return result, nil
}
