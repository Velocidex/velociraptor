package paths

import (
	"testing"

	"github.com/alecthomas/assert"
)

func TestPartitions(t *testing.T) {
	path := "c.3a1be9ef7a3549b0c.3a1be9ef7a3549b0"

	// We partition path into 3 levels.
	assert.Equal(t, splitTermToParts(path), []string{
		"c.3a", "1b", "e9ef7a3549b0c.3a1be9ef7a3549b0"})
}
