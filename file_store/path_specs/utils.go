package path_specs

import (
	"strings"

	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
)

func CleanPathForZip(path_spec api.FSPathSpec, client_id, hostname string) string {
	components := path_spec.Components()
	hostname = utils.SanitizeString(hostname)
	result := make([]string, 0, len(components))
	for _, component := range components {
		// Replace any client id with hostnames
		if component == client_id {
			component = hostname
		}

		result = append(result, utils.SanitizeString(component))
	}

	// Zip files should not have absolute paths
	return strings.Join(result, "/") + api.GetExtensionForFilestore(
		path_spec, path_spec.Type())
}
