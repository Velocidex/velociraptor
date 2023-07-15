package glob

import (
	"sort"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
)

// A FileInfo that reports the globs that matched.
type GlobHit struct {
	accessors.FileInfo

	globs []string
}

func (self GlobHit) Data() *ordereddict.Dict {
	data := self.FileInfo.Data()
	if data == nil {
		data = ordereddict.NewDict()
	}
	return data.Set("globs", self.globs)
}

func NewGlobHit(base accessors.FileInfo, globs []string) *GlobHit {
	return &GlobHit{
		FileInfo: base,
		globs:    globs,
	}
}

// Keeps track of the hits in a directory.
type dirHits struct {
	hits map[string]*GlobHit
}

// Merge a hit on a glob with existing hits in this directory.
func (self *dirHits) mergeHit(basename string, hit *GlobHit) {

	existing, ok := self.hits[basename]
	if !ok {
		existing = hit
		self.hits[basename] = existing

	} else {
		// Merge the globs for both matches
		existing.globs = append(existing.globs, hit.globs...)
	}
}

// Return all the hits
func (self *dirHits) getHits() []*GlobHit {

	// Sort dict by basename
	type tmpType struct {
		base     string
		fileinfo *GlobHit
	}

	tmp := make([]tmpType, 0, len(self.hits))
	for k, v := range self.hits {
		tmp = append(tmp, tmpType{base: k, fileinfo: v})
	}

	// Sort the results alphabetically.
	sort.Slice(tmp, func(i, j int) bool {
		return -1 == strings.Compare(tmp[i].base, tmp[j].base)
	})

	results := make([]*GlobHit, 0, len(tmp))
	for _, t := range tmp {
		results = append(results, t.fileinfo)
	}

	return results
}

func newDirHits() *dirHits {
	return &dirHits{
		hits: make(map[string]*GlobHit),
	}
}
