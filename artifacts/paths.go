package artifacts

import "strings"

func NameToPath(name string) string {
	return "/" + strings.Replace(name, ".", "/", -1) + ".yaml"
}
