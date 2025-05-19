package utils

import (
	"strings"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

// Normalize the base path. If base path is not specified or / return
// "". Otherwise ensure base path has a leading / and no following /
func GetBasePath(config_obj *config_proto.Config, parts ...string) string {
	frontend_service, err := services.GetFrontendManager(config_obj)
	if err != nil {
		return "/"
	}
	base, _ := frontend_service.GetBaseURL(config_obj)

	args := append([]string{base.Path}, parts...)
	base.Path = Join(args...)
	if base.Path == "/" {
		return ""
	}

	return base.Path
}

// Return the base directory (with the trailing /) for the base path
func GetBaseDirectory(config_obj *config_proto.Config) string {
	base := GetBasePath(config_obj)
	return strings.TrimSuffix(base, "/") + "/"
}

// Returns the fully qualified URL to the API endpoint.
func GetPublicURL(config_obj *config_proto.Config, parts ...string) string {
	frontend_service, err := services.GetFrontendManager(config_obj)
	if err != nil {
		return "/"
	}
	base, err := frontend_service.GetBaseURL(config_obj)
	if err != nil {
		return ""
	}

	args := append([]string{base.Path}, parts...)
	base.Path = Join(args...)
	return base.String()
}

// Returns the absolute public URL referring to all the parts
func PublicURL(config_obj *config_proto.Config, parts ...string) string {
	frontend_service, err := services.GetFrontendManager(config_obj)
	if err != nil {
		return "/"
	}
	base, err := frontend_service.GetBaseURL(config_obj)
	if err != nil {
		return "/"
	}
	args := append([]string{base.Path}, parts...)
	base.Path = Join(args...)
	return base.String()
}

// Join all parts of the URL to make sure that there is only a single
// / between them regardless of if they have leading or trailing /.
// Ensure the url starts with / unless it is an absolute URL starting
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
