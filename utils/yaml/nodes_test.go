package yaml

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetYamlNodesBounds(t *testing.T) {
	const doc = `
name: Sample
sources:
  - query: SELECT 1 FROM scope()
  - query: SELECT 2 FROM scope()
empty: []
`
	var parsed Node
	require.NoError(t, Unmarshal([]byte(doc), &parsed))
	require.NotEmpty(t, parsed.Content)
	root := parsed.Content[0]

	var nodes []NodeContext
	require.True(t, GetYamlNodes(root, root, strings.Split("sources.0.query", "."), &nodes))
	require.True(t, GetYamlNodes(root, root, strings.Split("sources.1.query", "."), &nodes))
	require.Len(t, nodes, 2)
	require.Equal(t, "SELECT 1 FROM scope()", nodes[0].Value)
	require.Equal(t, "SELECT 2 FROM scope()", nodes[1].Value)

	var all []NodeContext
	require.True(t, GetYamlNodes(root, root, strings.Split("sources.[].query", "."), &all))
	require.Len(t, all, 2)

	for _, invalidPath := range []string{
		"sources.-1.query",
		"sources.2.query",
		"sources.99.query",
		"empty.0",
		"name.0",
	} {
		var out []NodeContext
		require.False(t, GetYamlNodes(root, root, strings.Split(invalidPath, "."), &out), invalidPath)
		require.Empty(t, out, invalidPath)
	}

	var nilOut []NodeContext
	require.False(t, GetYamlNodes(nil, nil, []string{"sources"}, &nilOut))
	require.Empty(t, nilOut)
}
