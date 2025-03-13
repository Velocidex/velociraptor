package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/alecthomas/participle"
	"gopkg.in/yaml.v3"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/reformat"
)

var (
	invalidNode = errors.New("invalidNode")
)

type mutation struct {
	original_start_line, original_end_line int
	replacement                            []string
	err                                    error
}

type nodeContext struct {
	*yaml.Node

	parent *yaml.Node
}

// The yaml library emits nodes in an incosistent way which makes them
// hard to navigate. This function reorders the nodes into a proper
// document structure and fetches the relevant node.
func getYamlNodes(node, parent *yaml.Node,
	components []string, nodes *[]nodeContext) bool {

	if len(components) == 0 {
		*nodes = append(*nodes, nodeContext{
			Node:   node,
			parent: parent,
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
			if getYamlNodes(c, node, components[1:], nodes) {
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
		return getYamlNodes(node.Content[idx], node, components[1:], nodes)
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
				return getYamlNodes(node.Content[i+1], node, components[1:], nodes)
			}
		}
		// Didnt find it
		return false
	}

	return false
}

var VQLPaths = []string{
	"sources.[].query",
	"export",
	"precondition",
	"sources.[].precondition",
}

func getAllMutations(root *yaml.Node) (res []mutation, err error) {
	var nodes []nodeContext

	for _, p := range VQLPaths {
		getYamlNodes(root, root, strings.Split(p, "."), &nodes)
	}

	for _, n := range nodes {
		// We only reformat literal style nodes
		if n.Style != yaml.LiteralStyle {
			continue
		}

		m, err := reformatNode(n)
		if err != nil {
			return nil, err
		}
		res = append(res, m)
	}

	// sort mutations by original_start_line
	sort.Slice(res, func(i, j int) bool {
		return res[i].original_start_line < res[j].original_start_line
	})

	return res, nil
}

func reformatNode(vql_node nodeContext) (m mutation, err error) {
	scope := vql_subsystem.MakeScope()
	reformatted, err := reformat.ReFormatVQL(
		scope, vql_node.Value, vfilter.DefaultFormatOptions)
	if err != nil {
		line := 0
		message := err.Error()
		perr, ok := err.(participle.Error)
		if ok {
			line = perr.Token().Pos.Line
			message = perr.Message()
		}
		// Error should be reported to the GUI
		return m, fmt.Errorf("While parsing VQL at line %v: %v",
			vql_node.Line+line, message)
	}

	reformatted = strings.TrimSpace(reformatted)

	lines := []string{}
	for _, l := range strings.Split(reformatted, "\n") {
		lines = append(lines, l)
	}

	// Indent this block to the start of the previous block
	indented := []string{}
	ind := strings.Repeat(" ", vql_node.parent.Column+2)
	for _, l := range lines {
		indented = append(indented, ind+l)
	}

	indented = append(indented, "")

	return mutation{
		original_start_line: vql_node.Line,
		original_end_line: vql_node.Line +
			len(strings.Split(vql_node.Value, "\n")) - 1,
		replacement: indented,
	}, nil
}

func applyMutations(text string, mu []mutation) (string, error) {
	if len(mu) == 0 {
		return text, nil
	}
	lines := strings.Split(text, "\n")
	result := []string{}
	current_mu := mu[0]
	current_mu_idx := 0

	for i := 0; i < len(lines); {
		if i < current_mu.original_start_line {
			result = append(result, lines[i])
			i++
			continue
		}

		if i == current_mu.original_start_line {
			result = append(result, current_mu.replacement...)
			i = current_mu.original_end_line
			if current_mu_idx+1 >= len(mu) {
				// No more mutations, just copy the rest and return
				result = append(result, lines[i+1:]...)
				return strings.Join(result, "\n"), nil
			}
			current_mu_idx++
			current_mu = mu[current_mu_idx]
			if current_mu.err != nil {
				return text, current_mu.err
			}
		}
	}

	return strings.Join(result, "\n"), nil
}

func reformatVQL(in string) (string, error) {
	var node yaml.Node
	err := yaml.Unmarshal([]byte(in), &node)
	if err != nil {
		return "", err
	}

	if len(node.Content) == 0 {
		return in, nil
	}
	mutations, err := getAllMutations(node.Content[0])
	if err != nil {
		return "", err
	}

	// Now apply the mutations
	return applyMutations(in, mutations)
}

func (self *RepositoryManager) ReformatVQL(
	ctx context.Context, artifact_yaml string) (string, error) {

	return reformatVQL(artifact_yaml)
}
