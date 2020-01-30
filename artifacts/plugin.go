/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package artifacts

// This allows to run an artifact as a plugin.
import (
	"context"
	"fmt"
	"strings"

	"github.com/Velocidex/ordereddict"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type ArtifactRepositoryPlugin struct {
	repository *Repository
	children   map[string]vfilter.PluginGeneratorInterface
	prefix     []string
	leaf       *artifacts_proto.Artifact
	mock       []vfilter.Row
}

func (self *ArtifactRepositoryPlugin) SetMock(mock []vfilter.Row) {
	self.mock = mock
}

func (self *ArtifactRepositoryPlugin) Print() {
	var children []string
	for k := range self.children {
		children = append(children, k)
	}
	fmt.Printf("prefix '%v', Children %v, Leaf %v\n",
		self.prefix, children, self.leaf != nil)
	for _, v := range self.children {
		v.(*ArtifactRepositoryPlugin).Print()
	}
}

// Define vfilter.PluginGeneratorInterface
func (self *ArtifactRepositoryPlugin) Call(
	ctx context.Context, scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		if self.mock != nil {
			for _, row := range self.mock {
				output_chan <- row
			}

			return
		}

		if self.leaf == nil {
			scope.Log("Artifact %s not found", strings.Join(self.prefix, "."))
			return
		}

		artifact_definition := self.leaf
		v, pres := args.Get("source")
		if pres {
			lazy_v, ok := v.(vfilter.LazyExpr)
			if ok {
				v = lazy_v.Reduce()
			}

			source, ok := v.(string)
			if !ok {
				scope.Log("Source must be a string")
				return
			}

			artifact_definition, pres = self.repository.Get(
				self.leaf.Name + "/" + source)
			if !pres {
				scope.Log("Source %v not found in artifact %v",
					source, self.leaf.Name)
				return
			}
		}

		request := &actions_proto.VQLCollectorArgs{}
		err := self.repository.Compile(artifact_definition, request)
		if err != nil {
			scope.Log("Artifact %s invalid: %s",
				strings.Join(self.prefix, "."), err.Error())
			return
		}

		// We create a child scope for evaluating the artifact.
		env := ordereddict.NewDict()
		for _, request_env := range request.Env {
			env.Set(request_env.Key, request_env.Value)
		}

		// Allow the args to override the artifact defaults.
		for k, v := range *args.ToDict() {
			if k == "source" {
				continue
			}
			_, pres := env.Get(k)
			if !pres {
				scope.Log(fmt.Sprintf(
					"Unknown parameter %s provided to artifact %v",
					k, strings.Join(self.prefix, ".")))
				return
			}

			lazy_v, ok := v.(vfilter.LazyExpr)
			if ok {
				v = lazy_v.Reduce()
			}
			env.Set(k, v)
		}

		child_scope := scope.Copy().AppendVars(env)
		for _, query := range request.Query {
			vql, err := vfilter.Parse(query.VQL)
			if err != nil {
				scope.Log("Artifact %s invalid: %s",
					strings.Join(self.prefix, "."),
					err.Error())
				return
			}

			child_chan := vql.Eval(ctx, child_scope)
			for {
				row, ok := <-child_chan
				// This query is done - do the
				// next one.
				if !ok {
					break
				}
				dict_row := vql_subsystem.RowToDict(scope, row)
				if query.Name != "" {
					dict_row.Set("_Source", query.Name)
				}

				output_chan <- dict_row
			}
		}

	}()
	return output_chan
}

func (self *ArtifactRepositoryPlugin) Name() string {
	return strings.Join(self.prefix, ".")
}

func (self *ArtifactRepositoryPlugin) Info(
	scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: self.Name(),
		Doc:  "A pseudo plugin for accessing the artifacts repository from VQL.",
	}
}

// Define Associative protocol.
type _ArtifactRepositoryPluginAssociativeProtocol struct{}

func _getArtifactRepositoryPlugin(a vfilter.Any) *ArtifactRepositoryPlugin {
	switch t := a.(type) {
	case ArtifactRepositoryPlugin:
		return &t

	case *ArtifactRepositoryPlugin:
		return t

	default:
		return nil
	}
}

func (self _ArtifactRepositoryPluginAssociativeProtocol) Applicable(
	a vfilter.Any, b vfilter.Any) bool {
	if _getArtifactRepositoryPlugin(a) == nil {
		return false
	}

	switch b.(type) {
	case string:
		break
	default:
		return false
	}

	return true
}

func (self _ArtifactRepositoryPluginAssociativeProtocol) GetMembers(
	scope *vfilter.Scope, a vfilter.Any) []string {
	var result []string

	value := _getArtifactRepositoryPlugin(a)
	if value != nil {
		for k := range value.children {
			result = append(result, k)
		}
	}
	return result
}

func (self _ArtifactRepositoryPluginAssociativeProtocol) Associative(
	scope *vfilter.Scope, a vfilter.Any, b vfilter.Any) (vfilter.Any, bool) {
	value := _getArtifactRepositoryPlugin(a)
	if value == nil {
		return nil, false
	}

	key, _ := b.(string)
	child, pres := value.children[key]
	return child, pres
}

func NewArtifactRepositoryPlugin(
	repository *Repository, prefix []string) vfilter.PluginGeneratorInterface {
	result := &ArtifactRepositoryPlugin{
		repository: repository,
		children:   make(map[string]vfilter.PluginGeneratorInterface),
		prefix:     prefix,
	}

	for _, name := range repository.List() {
		components := strings.Split(name, ".")
		if len(components) < len(prefix) ||
			!utils.SlicesEqual(components[:len(prefix)], prefix) {
			continue
		}

		components = components[len(prefix):]

		// We are at a leaf node.
		if len(components) == 0 {
			artifact, _ := repository.Get(name)
			result.leaf = artifact
			return result
		}

		_, pres := result.children[components[0]]
		if !pres {
			result.children[components[0]] = NewArtifactRepositoryPlugin(
				repository, append(prefix, components[0]))
		}
	}

	return result
}
