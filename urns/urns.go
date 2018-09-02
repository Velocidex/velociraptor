package urns

import (
	"path"
	"strings"
)

func BuildURN(elem ...string) string {
	base_path := path.Join(elem...)
	if strings.HasPrefix(elem[0], "aff4:") {
		return base_path
	}
	return "aff4:/" + base_path
}
