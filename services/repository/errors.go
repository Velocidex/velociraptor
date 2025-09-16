package repository

import (
	"strings"

	"github.com/alecthomas/participle"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/utils/yaml"
)

func reportError(errToReport error,
	artifact *artifacts_proto.Artifact, field string, idx int) error {

	var node yaml.Node
	err := yaml.Unmarshal([]byte(artifact.Raw), &node)
	if err == nil || len(node.Content) > 0 {
		root := node.Content[0]
		var nodes []yaml.NodeContext
		yaml.GetYamlNodes(root, root, strings.Split(field, "."), &nodes)

		if len(nodes) >= idx+1 {
			line := nodes[idx].Line
			err, ok := errToReport.(participle.UnexpectedTokenError)
			if ok {
				// Correct the line number of the error.
				err.Unexpected.Pos.Line += line
				return err
			}
		}
	}
	return errToReport
}
