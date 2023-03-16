package authenticators

import (
	"strings"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

// Normalize the base path. If base path is not specified or / return
// "". Otherwise ensure base path has a leading / and no following /
func GetBasePath(config_obj *config_proto.Config) string {
	if config_obj.GUI == nil || config_obj.GUI.BasePath == "" {
		return ""
	}

	bare := strings.TrimSuffix(config_obj.GUI.BasePath, "/")
	bare = strings.TrimPrefix(bare, "/")
	if bare == "" {
		return ""
	}
	return "/" + bare
}

// Return the base directory (with the trailing /) for the base path
func GetBaseDirectory(config_obj *config_proto.Config) string {
	return GetBasePath(config_obj) + "/"
}

// Ensure public URL start and ends with /
func GetPublicURL(config_obj *config_proto.Config) string {
	bare := strings.TrimSuffix(config_obj.GUI.PublicUrl, "/")
	return bare + "/"
}

// Join all parts of the URL to make sure that there is only a single
// / between them regardless of if they have leading or trailing /.
// Ensure the url starts withv / unless it is an absolute URL starting
// with http If the final part ends with / preserve that to refer to a
// directory.
func Join(parts ...string) string {
	if len(parts) == 0 {
		return "/"
	}

	result := []string{}
	for _, p := range parts {
		p = strings.TrimPrefix(p, "/")
		p = strings.TrimSuffix(p, "/")
		if p != "" {
			result = append(result, p)
		}
	}

	res := strings.Join(result, "/")
	// If the last part ends with / preserve that
	if strings.HasSuffix(parts[len(parts)-1], "/") {
		res += "/"
	}

	// Ensure the URL starts with  /
	if !strings.HasPrefix(res, "/") && !strings.HasPrefix(res, "http") {
		res = "/" + res
	}

	return res
}

func Homepage(config_obj *config_proto.Config) string {
	base := GetBasePath(config_obj)
	return Join(base, "/app/index.html")
}
