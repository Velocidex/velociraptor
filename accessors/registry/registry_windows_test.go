// +build windows

package registry

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/sebdah/goldie"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func TestRegistryFilesystemAccessor(t *testing.T) {
	accessor := &RegFileSystemAccessor{}

	ls := func(path string, filter string) []string {
		filter_re := regexp.MustCompile(filter)

		children, err := accessor.ReadDir(path)
		assert.NoError(t, err)

		results := []string{}
		for _, c := range children {
			path := fmt.Sprintf("%v - %v %v", c.FullPath(),
				c.Mode(), c.Data())
			if filter != "" && !filter_re.MatchString(path) {
				continue
			}
			results = append(results, path)
		}
		return results
	}

	results := ordereddict.NewDict()
	results.Set("Root listing", ls("/", "."))
	results.Set("Deep key", ls("HKLM/SYSTEM/CurrentControlSet/Control/CMF", "CompressedSegments|LatestIndex"))

	goldie.Assert(t, "TestRegistrtFilesystemAccessor",
		json.MustMarshalIndent(results))
}
