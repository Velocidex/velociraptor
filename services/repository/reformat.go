package repository

import (
	"context"
	"strings"

	"gopkg.in/yaml.v3"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/reformat"
)

type mutation struct {
	original_start_line, original_end_line int
	replacement                            []string
}

func vqlNode(node *yaml.Node) bool {
	switch node.Value {
	case "query", "export", "precondition":
		return true
	}
	return false
}

func findAllQueries(root *yaml.Node, mu *[]mutation) {
	for i, c := range root.Content {
		if vqlNode(c) && len(root.Content) > i {
			vql_node := root.Content[i+1]

			// We only reformat literal style nodes
			if vql_node.Style != yaml.LiteralStyle {
				continue
			}

			scope := vql_subsystem.MakeScope()
			reformatted, err := reformat.ReFormatVQL(
				scope, vql_node.Value, vfilter.DefaultFormatOptions)
			if err != nil {
				continue
			}
			lines := []string{}
			for _, l := range strings.Split(reformatted, "\n") {
				if strings.TrimSpace(l) == "" {
					continue
				}
				lines = append(lines, l)
			}

			// Indent this block to the start of the previous block
			indented := []string{}
			ind := strings.Repeat(" ", c.Column)
			for _, l := range lines {
				indented = append(indented, ind+l)
			}
			// Add an extra blank space after the VQL block.
			indented = append(indented, "")

			*mu = append(*mu, mutation{
				original_start_line: vql_node.Line,
				original_end_line: vql_node.Line +
					len(strings.Split(vql_node.Value, "\n")) - 1,
				replacement: indented,
			})
		}
		findAllQueries(c, mu)
	}
}

func applyMutations(text string, mu []mutation) string {
	if len(mu) == 0 {
		return text
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
				return strings.Join(result, "\n")
			}
			current_mu_idx++
			current_mu = mu[current_mu_idx]
		}
	}
	return strings.Join(result, "\n")
}

func reformatVQL(in string) (string, error) {
	var node yaml.Node
	err := yaml.Unmarshal([]byte(in), &node)
	if err != nil {
		return "", err
	}

	mutations := []mutation{}
	findAllQueries(&node, &mutations)

	// Now apply the mutations
	return applyMutations(in, mutations), nil
}

func (self *RepositoryManager) ReformatVQL(
	ctx context.Context, artifact_yaml string) (string, error) {

	return reformatVQL(artifact_yaml)
}
