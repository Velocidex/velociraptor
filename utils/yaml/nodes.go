package yaml

import (
	"strconv"

	"gopkg.in/yaml.v3"
)

type Node = yaml.Node

var (
	LiteralStyle = yaml.LiteralStyle
)

func Unmarshal(in []byte, item interface{}) error {
	return yaml.Unmarshal(in, item)
}

type NodeContext struct {
	*yaml.Node

	Parent *yaml.Node
}

// The yaml library emits nodes in an incosistent way which makes them
// hard to navigate. This function reorders the nodes into a proper
// document structure and fetches the relevant node.
func GetYamlNodes(node, parent *yaml.Node,
	components []string, nodes *[]NodeContext) bool {

	if len(components) == 0 {
		*nodes = append(*nodes, NodeContext{
			Node:   node,
			Parent: parent,
		})
		return true
	}

	next := components[0]
	if next == "[]" {
		if node.Tag != "!!seq" {
			return false
		}
		res := false
		for _, c := range node.Content {
			if GetYamlNodes(c, node, components[1:], nodes) {
				res = true
			}
		}
		return res
	}

	idx, err := strconv.ParseInt(next, 0, 64)
	if err == nil {
		// It is not a sequence
		if node.Tag != "!!seq" ||
			// Sequence too short
			len(node.Content) < int(idx) {
			return false
		}

		// Child is found - keep going to the next component
		return GetYamlNodes(node.Content[idx], node, components[1:], nodes)
	}

	// Walk a mapping
	if node.Tag == "!!map" {
		// should not happen
		if len(node.Content)%2 != 0 {
			return false
		}

		// Maps are set up in node.Content as a list of key, value.
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i].Value
			if key == next {
				return GetYamlNodes(node.Content[i+1],
					node, components[1:], nodes)
			}
		}
		// Didnt find it
		return false
	}

	return false
}
