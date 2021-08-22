package paths

import (
	"strings"

	"www.velocidex.com/golang/velociraptor/file_store/api"
)

const (
	partition = 3
)

type IndexPathManager struct{}

func (self IndexPathManager) IndexTerm(term, entity string) api.DSPathSpec {
	return CLIENT_INDEX_URN.AddUnsafeChild(splitTermToParts(term + entity)...).
		AddUnsafeChild(entity)
}

// Returns a pathspec where walking the pathspec will return all the
// entities indexed under the same term.
func (self IndexPathManager) EnumerateTerms(term string) api.DSPathSpec {
	return CLIENT_INDEX_URN.AddUnsafeChild(splitTermToParts(term)...)
}

func (self IndexPathManager) TermPartitions(term string) []string {
	return splitTermToParts(term)
}

func NewIndexPathManager() *IndexPathManager {
	return &IndexPathManager{}
}

func splitTermToParts(term string) []string {
	// Lowercase the term so we can search case insensitive.
	term = strings.ToLower(term)

	// Partition the term into character groups
	parts := []string{}
	for i := 0; i < len(term); i += partition {
		left := i
		right := i + partition
		if right > len(term) {
			right = len(term)
		}
		parts = append(parts, term[left:right])
	}
	return parts
}
