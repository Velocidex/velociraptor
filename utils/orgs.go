package utils

import (
	"strings"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func ClientIdFromConfigObj(source string,
	config_obj *config_proto.Config) string {
	if config_obj.Client != nil {
		org_id := config_obj.OrgId
		if IsRootOrg(org_id) {
			return source
		}
		return source + "-" + org_id
	}

	return source
}

func ClientIdFromSourceAndOrg(source, org_id string) string {
	if IsRootOrg(org_id) {
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

func ClientIdFromSource(client_id string) string {
	parts := strings.Split(client_id, "-")
	return parts[0]
}

func IsRootOrg(org_id string) bool {
	return org_id == "" || org_id == "root"
}

func NormalizedOrgId(org_id string) string {
	if IsRootOrg(org_id) {
		return "root"
	}
	return org_id
}
func CompareOrgIds(a, b string) bool {
	if IsRootOrg(a) && IsRootOrg(b) {
		return true
	}
	return a == b
}

func OrgIdInList(org_id string, list []string) bool {
	for _, i := range list {
		if CompareOrgIds(org_id, i) {
			return true
		}
	}
	return false
}

func GetOrgId(config_obj *config_proto.Config) string {
	return NormalizedOrgId(config_obj.OrgId)
}
