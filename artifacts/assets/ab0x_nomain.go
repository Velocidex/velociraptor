//go:build !velomain
// +build !velomain

package assets

import "www.velocidex.com/golang/velociraptor/utils"

var (
	FileDocsReferencesVqlYaml = []byte("")
	Inventory                 = map[string][]byte{}
)

// ReadFile is adapTed from ioutil
func ReadFile(path string) ([]byte, error) {
	return nil, utils.NotImplementedError
}
