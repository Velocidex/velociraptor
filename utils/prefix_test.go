package utils

import (
	"testing"

	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func TestPrefix(t *testing.T) {
	tree := NewPrefixTree(true)
	tree.Add([]string{"C:", "Windows"})
	tree.Add([]string{"C:", "Users"})

	assert.True(t, tree.Present([]string{"C:", "Windows", "System32"}))
	assert.True(t, tree.Present([]string{"C:", "Windows"}))
	assert.False(t, tree.Present([]string{"C:", "Program Files"}))

	assert.True(t, tree.Present([]string{"C:"}))
	assert.True(t, tree.Present([]string{"C:", "Users"}))
}
