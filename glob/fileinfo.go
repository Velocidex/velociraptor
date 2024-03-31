package glob

import (
	"sort"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/utils"
)

// A FileInfo that reports the globs that matched.
type GlobHit struct {
	accessors.FileInfo

	globs []string
}

// Report all matching globs
func (self GlobHit) Globs() []string {
	ret := make([]string, 0, len(self.globs))

	// Should be short so O(1) is OK
	for _, i := range self.globs {
		if !utils.InString(ret, i) {
			ret = append(ret, i)
		}
	}
	return ret
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

// Merge a hit on a basename with existing hits in this directory.
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

// Return all the hits sorted by increasing lexical basename
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
		return tmp[i].fileinfo.Name() < tmp[j].fileinfo.Name()
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
