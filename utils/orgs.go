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

// Builds a standard client id for the client - this includes the org
// id and the client id: e.g. C.1234-O123
func ClientIdFromSourceAndOrg(source, org_id string) string {
	source = ClientIdFromSource(source)
	if IsRootOrg(org_id) {
		return source
	}

	return source + "-" + org_id
}

// Extracts the org id from the client id
func OrgIdFromClientId(client_id string) string {
	parts := strings.SplitN(client_id, "-", 2)
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}

// Extracts the pure client id from the client_id dropping the org id.
func ClientIdFromSource(client_id string) string {
	parts := strings.Split(client_id, "-")
	return parts[0]
}

// Is the root id representing the Root org.
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
