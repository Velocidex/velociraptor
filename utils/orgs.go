package utils

import (
	"strings"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func ClientIdFromConfigObj(source string,
	config_obj *config_proto.Config) string {
	if config_obj.Client != nil {
		org_id := config_obj.OrgId
		return source + "-" + org_id
	}
	return source
}

func ClientIdFromSourceAndOrg(source, org_id string) string {
	return source + "-" + org_id
}

func OrgIdFromClientId(client_id string) string {
	parts := strings.Split(client_id, "-")
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}
