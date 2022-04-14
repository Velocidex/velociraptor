package paths

import (
	"fmt"
	"strings"
	"time"

	"www.velocidex.com/golang/velociraptor/file_store/api"
)

var (
	// Since v0.6.3 we only store client id in the index. This is a
	// hex encoded random value of the form "C.XXYYZZ"
	partitions = []int{
		// Partition 1: "C.XX" - first hex digit has density of < 256
		// Partition 2: "YY" Has density of < 256
		// Partition 3: Rest of digits are randomly distributed. With
		//   density of 256 up to 1.6m clients.
		4, 2, 100,
	}
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

func (self IndexPathManager) Snapshot() api.FSPathSpec {
	return CLIENT_INDEX_URN.AddChild("snapshot").
		AsFilestorePath().
		SetType(api.PATH_TYPE_FILESTORE_JSON)
}

func (self IndexPathManager) SnapshotTimed() api.FSPathSpec {
	now := time.Now().UTC()
	day_name := fmt.Sprintf("%d-%02d-%02d", now.Year(),
		now.Month(), now.Day())

	return CLIENT_INDEX_URN.AddChild("snapshots", day_name).
		AsFilestorePath().
		SetType(api.PATH_TYPE_FILESTORE_JSON)
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
	i := 0
	for j := 0; j < len(partitions); j++ {
		partition := partitions[j]
		left := i
		right := i + partition
		if right > len(term) {
			right = len(term)
		}
		parts = append(parts, term[left:right])

		i += partition
		if i >= len(term) {
			break
		}

	}
	return parts
}
