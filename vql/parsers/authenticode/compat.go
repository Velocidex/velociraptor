// +build !windows !amd64

package authenticode

import (
	"os"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

// Placeholder for non windows system. This will mostly work except
// verification wont be available.

func VerifyFileSignature(normalized_path string) string {
	return "Unknown (No API access)"
}

func VerifyCatalogSignature(
	config_obj *config_proto.Config,
	fd *os.File, normalized_path string,
	output *ordereddict.Dict) (string, error) {
	return "Unknown (No API access)", nil
}

func ParseCatFile(cat_file string, output *ordereddict.Dict, verbose bool) error {
	return nil
}

func GetNativePath(path string) string {
	return path
}
