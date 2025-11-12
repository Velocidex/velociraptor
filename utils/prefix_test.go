package utils

import (
	"testing"

	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func TestPrefix(t *testing.T) {
	tree := NewPrefixTree(CaseInsensitivePrefix)
	tree.Add([]string{"C:", "Windows"})
	tree.Add([]string{"C:", "Users"})
	tree.Add([]string{"C:", "Users", "Administrator"})

	var match bool
	var depth int

	// A shorter path is not contained.
	match, depth = tree.Present([]string{"C:"})
	assert.False(t, match)

	match, depth = tree.Present([]string{})
	assert.False(t, match)

	// Matching a deep path will return the length of the prefix
	// match.
	match, depth = tree.Present([]string{"C:", "Windows", "System32", "WinEvt", "Logs"})
	assert.True(t, match)
	assert.Equal(t, depth, 2)

	// Exact match on a prefix
	match, depth = tree.Present([]string{"C:", "Windows"})
	assert.True(t, match)
	assert.Equal(t, depth, 2)

	// A different directory than the prefixes
	match, depth = tree.Present([]string{"C:", "Program Files"})
	assert.False(t, match)

	// Shorter directories do not match
	match, depth = tree.Present([]string{"C:"})
	assert.False(t, match)

	// Exact length match
	match, depth = tree.Present([]string{"c:", "users"})
	assert.True(t, match)
	assert.Equal(t, depth, 2)

	// Depth first search gives the longest prefix that matches.
	match, depth = tree.Present([]string{"c:", "users", "administrator", "documents"})
	assert.True(t, match)
	assert.Equal(t, depth, 3)
}
