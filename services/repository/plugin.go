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
package repository

// This allows to run an artifact as a plugin.
import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Velocidex/ordereddict"
	errors "github.com/pkg/errors"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

const (
	MAX_STACK_DEPTH = 10
)

type ArtifactRepositoryPlugin struct {
	repository *Repository
	children   map[string]vfilter.PluginGeneratorInterface
	prefix     []string
	leaf       *artifacts_proto.Artifact
	mock       []vfilter.Row
	wg         *sync.WaitGroup
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
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	self.wg.Add(1)
	go func() {
		defer self.wg.Done()
		defer close(output_chan)

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("Failed to get config_obj")
			return
		}

		// If the ctx is done do nothing.
		if self.mock != nil {
			for _, row := range self.mock {
				select {
				case <-ctx.Done():
					return
				case output_chan <- row:
				}
			}

			return
		}

		if self.leaf == nil {
			scope.Log("Artifact %s not found", strings.Join(self.prefix, "."))
			return
		}

		artifact_name := self.leaf.Name
		source := ""

		v, pres := args.Get("source")
		if pres {
			lazy_v, ok := v.(types.LazyExpr)
			if ok {
				v = lazy_v.Reduce()
			}

			source, ok = v.(string)
			if !ok {
				scope.Log("Source must be a string")
				return
			}
		}

		artifact_definition, pres := self.repository.Get(config_obj, artifact_name)
		// Should never really happen.
		if !pres {
			scope.Log("Artifact %v not found", self.leaf.Name)
			return
		}

		// The artifact may have multiple sources, but we need
		// to only call a single source here because we want
		// to have consistent result set. So we make a copy of
		// the artifact with just the sources we want.
		our_artifact := &artifacts_proto.Artifact{
			Name:       artifact_definition.Name,
			Tools:      artifact_definition.Tools,
			Parameters: []*artifacts_proto.ArtifactParameter{},
		}

		// Find the correct source the caller wanted.
		for _, source_def := range artifact_definition.Sources {
			if source_def.Name == source {
				our_artifact.Sources = append(
					our_artifact.Sources, source_def)
				break
			}
		}

		if len(our_artifact.Sources) == 0 {
			scope.Log("Calling Artifact %s with source %s: not found.",
				strings.Join(self.prefix, "."), source)
			return
		}

		// Remove the parameters we are passing from the
		// definition to avoid type conversion. Default values
		// must be converted from string to VQL types, but
		// here we are already passing proper types in the
		// scope - therefore we need to prevent the compiler
		// from generating type conversion.
		known_parameters := make(map[string]bool)
		for _, param := range artifact_definition.Parameters {
			known_parameters[param.Name] = true

			_, pres := args.Get(param.Name)
			if !pres {
				our_artifact.Parameters = append(
					our_artifact.Parameters, param)
			}
		}

		launcher, err := services.GetLauncher()
		if err != nil {
			return
		}

		// This will generate a singe request for our new
		// single source artifact with reduced parameters. We
		// will then inject our parameters as into the scope
		// to populate the plugin.
		request, err := launcher.GetVQLCollectorArgs(ctx, config_obj,
			self.repository, our_artifact, &flows_proto.ArtifactSpec{}, false)
		if err != nil {
			scope.Log("Artifact %s invalid: %v",
				strings.Join(self.prefix, "."), err)
			return
		}

		// We create a child scope for evaluating the artifact.
		child_scope, err := self.copyScope(
			scope, self.leaf.Name)
		if err != nil {
			scope.Log("Error: %v", err)
			return
		}
		defer child_scope.Close()

		// Pass the args in the new scope.
		env := ordereddict.NewDict()
		for _, request_env := range request.Env {
			env.Set(request_env.Key, request_env.Value)
		}

		// Inject our parameters into the plugin environment.
		for _, k := range args.Keys() {
			if k == "source" {
				continue
			}

			v, _ := args.Get(k)

			// Check if the parameter is known.
			_, pres := known_parameters[k]
			if !pres {
				scope.Log(fmt.Sprintf(
					"Unknown parameter %s provided to artifact %v",
					k, strings.Join(self.prefix, ".")))
				return
			}

			// If the parameter is lazy, reduce it.
			lazy_v, ok := v.(types.LazyExpr)
			if ok {
				v = lazy_v.Reduce()
			}

			env.Set(k, v)
		}

		// Add the scope args
		child_scope.AppendVars(env)

		for _, query := range request.Query {
			vql, err := vfilter.Parse(query.VQL)
			if err != nil {
				scope.Log("Artifact %s invalid: %s",
					strings.Join(self.prefix, "."),
					err.Error())
				return
			}

			for row := range vql.Eval(ctx, child_scope) {
				dict_row := vfilter.RowToDict(ctx, child_scope, row)
				if query.Name != "" {
					dict_row.Set("_Source", query.Name)
				}
				select {
				case <-ctx.Done():
					return
				case output_chan <- dict_row:
				}
			}
		}

	}()
	return output_chan
}

// Create a mostly new scope for executing the new artifact but copy
// over some important global variables.
func (self *ArtifactRepositoryPlugin) copyScope(
	scope vfilter.Scope, my_name string) (
	vfilter.Scope, error) {
	env := ordereddict.NewDict()
	for _, field := range []string{
		vql_subsystem.ACL_MANAGER_VAR,
		vql_subsystem.CACHE_VAR,
		constants.SCOPE_MOCK,
		constants.SCOPE_CONFIG,
		constants.SCOPE_SERVER_CONFIG,
		constants.SCOPE_THROTTLE,
		constants.SCOPE_ROOT,
		constants.SCOPE_UPLOADER} {
		value, pres := scope.Resolve(field)
		if pres {
			env.Set(field, value)
		}
	}

	// Copy the old stack and push our name at the top of it so we
	// can produce a nice stack trace on overflow.
	stack_any, pres := scope.Resolve(constants.SCOPE_STACK)
	if !pres {
		env.Set(constants.SCOPE_STACK, []string{my_name})
	} else {
		child_stack, ok := stack_any.([]string)
		if ok {
			// Make a copy of the stack.
			stack := append([]string{my_name}, child_stack...)
			if len(stack) > MAX_STACK_DEPTH {
				return nil, errors.New("Stack overflow: " +
					strings.Join(stack, ", "))
			}
			env.Set(constants.SCOPE_STACK, stack)
		}
	}

	result := scope.Copy()
	result.ClearContext()
	result.AppendVars(env)

	return result, nil
}

func (self *ArtifactRepositoryPlugin) Name() string {
	return strings.Join(self.prefix, ".")
}

func (self *ArtifactRepositoryPlugin) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
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
	scope vfilter.Scope, a vfilter.Any) []string {
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
	scope vfilter.Scope, a vfilter.Any, b vfilter.Any) (vfilter.Any, bool) {

	value := _getArtifactRepositoryPlugin(a)
	if value == nil {
		return nil, false
	}

	key, _ := b.(string)
	child, pres := value.children[key]
	return child, pres
}

func NewArtifactRepositoryPlugin(
	wg *sync.WaitGroup,
	repository *Repository) vfilter.PluginGeneratorInterface {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	if repository.artifact_plugin != nil {
		return repository.artifact_plugin
	}

	name_listing := repository.list()

	// Cache it for next time and return it.
	repository.artifact_plugin = _NewArtifactRepositoryPlugin(
		repository, wg, name_listing, nil)

	return repository.artifact_plugin
}

func _NewArtifactRepositoryPlugin(
	repository *Repository,
	wg *sync.WaitGroup,
	name_listing []string,
	prefix []string) vfilter.PluginGeneratorInterface {

	result := &ArtifactRepositoryPlugin{
		repository: repository,
		wg:         wg,
		children:   make(map[string]vfilter.PluginGeneratorInterface),
		prefix:     prefix,
	}

	for _, name := range name_listing {
		components := strings.Split(name, ".")
		if len(components) < len(prefix) ||
			!utils.SlicesEqual(components[:len(prefix)], prefix) {
			continue
		}

		components = components[len(prefix):]

		// We are at a leaf node.
		if len(components) == 0 {
			artifact, _ := repository.get(name)
			result.leaf = artifact
			return result
		}

		_, pres := result.children[components[0]]
		if !pres {
			result.children[components[0]] = _NewArtifactRepositoryPlugin(
				repository, wg, name_listing, append(prefix, components[0]))
		}
	}

	return result
}
