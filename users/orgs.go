package users

import (
	"context"

	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

// List all the orgs the user can see.
func GetOrgs(
	ctx context.Context,
	principal string) []*api_proto.OrgRecord {

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return nil
	}

	result := []*api_proto.OrgRecord{}

	for _, org := range org_manager.ListOrgs() {
		org_config_obj, err := org_manager.GetOrgConfig(org.Id)
		if err != nil {
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
