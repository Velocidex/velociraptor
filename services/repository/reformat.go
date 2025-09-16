package repository

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/alecthomas/participle"
	"www.velocidex.com/golang/velociraptor/utils/yaml"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/reformat"
)

type mutation struct {
	original_start_line, original_end_line int
	replacement                            []string
	err                                    error
}

var VQLPaths = []string{
	"sources.[].query",
	"export",
	"precondition",
	"sources.[].precondition",
}

func getAllMutations(root *yaml.Node) (res []mutation, err error) {
	var nodes []yaml.NodeContext

	for _, p := range VQLPaths {
		yaml.GetYamlNodes(root, root, strings.Split(p, "."), &nodes)
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

func reformatNode(vql_node yaml.NodeContext) (m mutation, err error) {
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
	ind := strings.Repeat(" ", vql_node.Parent.Column+1)
	for _, l := range lines {
		indented = append(indented, ind+l)
	}

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
