package utils

import (
	"strings"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func ClientIdFromConfigObj(source string,
	config_obj *config_proto.Config) string {
	if config_obj.Client != nil {
		org_id := config_obj.OrgId
		if org_id == "" {
			return source
		}
		return source + "-" + org_id
	}

	return source
}

func ClientIdFromSourceAndOrg(source, org_id string) string {
	if org_id == "" {
		return source
	}

	return source + "-" + org_id
}

func OrgIdFromClientId(client_id string) string {
	parts := strings.Split(client_id, "-")
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}

func IsRootOrg(org_id string) bool {
	return org_id == "" || org_id == "root"
}
