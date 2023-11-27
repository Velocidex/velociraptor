package users

import (
	"context"
	"sort"

	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

// List all the orgs the user can see.
func (self *UserManager) GetOrgs(
	ctx context.Context, principal string) []*api_proto.OrgRecord {

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return nil
	}

	root_config_obj, err := org_manager.GetOrgConfig(services.ROOT_ORG_ID)
	if err != nil {
		return nil
	}

	// ORG_ADMINs can see everything so they have permissions in all
	// the orgs
	is_superuser, _ := services.CheckAccess(
		root_config_obj, principal, acls.ORG_ADMIN)

	result := []*api_proto.OrgRecord{}

	for _, org := range org_manager.ListOrgs() {
		org_config_obj, err := org_manager.GetOrgConfig(org.Id)
		if err != nil {
			continue
		}

		// ORG_ADMIN can see everything
		if is_superuser {
			result = append(result, org)
			continue
		}

		allowed, _ := services.CheckAccess(org_config_obj,
			principal, acls.SERVER_ADMIN)
		if allowed {
			result = append(result, org)
			continue
		}

		allowed, _ = services.CheckAccess(org_config_obj,
			principal, acls.READ_RESULTS)
		if allowed {
			result = append(result, &api_proto.OrgRecord{
				Id:   org.Id,
				Name: org.Name,
			})
			continue
		}
	}

	return result
}

// Fill in the orgs that the user has any permissions in.
func (self *UserManager) normalizeOrgList(
	ctx context.Context,
	user_record *api_proto.VelociraptorUser) {
	orgs := self.GetOrgs(ctx, user_record.Name)
	user_record.Orgs = nil

	// Fill in some information from the orgs but not everything.
	for _, org_record := range orgs {
		user_record.Orgs = append(user_record.Orgs, &api_proto.OrgRecord{
			Id:   org_record.Id,
			Name: org_record.Name,
		})
	}

	// Sort orgs for stable output
	sort.Slice(user_record.Orgs, func(i, j int) bool {
		return user_record.Orgs[i].Id < user_record.Orgs[j].Id
	})
}
