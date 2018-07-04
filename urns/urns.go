package urns

import (
	"path"
)

func BuildURN(elem ...string) string {
	joined_elements := []string{"/"}
	for _, i := range elem {
		joined_elements = append(joined_elements, i)
	}
	base_path := path.Join(joined_elements...)
	return "aff4:" + base_path
}
