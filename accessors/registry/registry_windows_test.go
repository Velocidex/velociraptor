// +build windows

package registry

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/sebdah/goldie"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func TestRegistrtFilesystemAccessor(t *testing.T) {
	accessor := &RegFileSystemAccessor{}

	ls := func(path string, filter string) []string {
		children, err := accessor.ReadDir(path)
		assert.NoError(t, err)

		results := []string{}
		for _, c := range children {
			path := fmt.Sprintf("%v - %v %v", c.FullPath(),
				c.Mode(), c.Data())
			if filter != "" && !strings.Contains(filter, path) {
				continue
			}
			results = append(results, path)
		}
		return results
	}

	results := ordereddict.NewDict()
	results.Set("Root listing", ls("/", ""))
	results.Set("Deep key", ls("HKLM/SYSTEM/CurrentControlSet/Control/CMF", ""))

	goldie.Assert(t, "TestRegistrtFilesystemAccessor",
		json.MustMarshalIndent(results))
}
