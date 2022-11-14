package users

import (
	"context"

	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	LIST_ALL_ORGS []string = nil
)

// List all the users visible to this principal.
// - If the principal is an ORG_ADMIN they can see all users
// - If the user is SERVER_ADMIN they will see the full user records.
// - Otherwise list all users belonging to the orgs in which the user
//   has at least read access. Only user names will be shown.
func ListUsers(
	ctx context.Context,
	principal string, orgs []string) ([]*api_proto.VelociraptorUser, error) {

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return nil, err
	}

	root_config_obj, err := org_manager.GetOrgConfig(services.ROOT_ORG_ID)
	if err != nil {
		return nil, err
	}

	user_manager := services.GetUserManager()
	users, err := user_manager.ListUsers(ctx)
	if err != nil {
		return nil, err
	}

	// Filter the users according to the access
	result := make([]*api_proto.VelociraptorUser, 0, len(users))
	type org_info struct {
		hidden       bool
		allowed      bool
		allowed_full bool
	}
	seen := make(map[string]org_info)

	for _, user := range users {
		// This is the record we will return.
		user_record := &api_proto.VelociraptorUser{
			Name: user.Name,
		}

		allowed_full := false
		returned_orgs := []*api_proto.OrgRecord{}

		for _, user_org := range user.Orgs {
			info, pres := seen[user_org.Id]
			if !pres {
				if len(orgs) > 0 && !utils.OrgIdInList(user_org.Id, orgs) {
					info.hidden = true
				} else {
					org_config_obj, err := org_manager.GetOrgConfig(user_org.Id)
					if err != nil {
						continue
					}

					// ORG_ADMINs can see everything
					info.allowed_full, _ = services.CheckAccess(
						root_config_obj, principal, acls.ORG_ADMIN)

					// Otherwise the user is an admin in their org
					if !info.allowed_full {
						info.allowed_full, _ = services.CheckAccess(org_config_obj,
							principal, acls.SERVER_ADMIN)
					}

					// If they just have reader access in their org
					// they only see the name.
					if !info.allowed_full {
						info.allowed, _ = services.CheckAccess(org_config_obj,
							principal, acls.READ_RESULTS)
					}
				}
				seen[user_org.Id] = info
			}

			if info.hidden {
				continue
			}

			// If we have full access, copy the entire record.
			if info.allowed_full {
				allowed_full = true

				// If we have only read access only copy the name.
			} else if !info.allowed {
				continue
			}

			returned_orgs = append(returned_orgs, user_org)
		}

		if len(returned_orgs) > 0 {
			if allowed_full {
				user_record.Orgs = returned_orgs
			}

			result = append(result, user_record)
		}
	}

	return result, nil
}
