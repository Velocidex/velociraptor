package repository

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

const (
	MAX_STACK_DEPTH = 10
)

type ArtifactRepositoryPlugin struct {
	prefix     []string
	repository services.Repository
	config_obj *config_proto.Config

	mock_call_count map[string]int
	mocks           map[string][]vfilter.Row
}

func (self *ArtifactRepositoryPlugin) SetMock(
	artifact string, mock []vfilter.Row) {
	self.mocks[artifact] = mock
	self.mock_call_count[artifact] = 0
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

func (self *ArtifactRepositoryPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		var artifact *artifacts_proto.Artifact

		artifact_name := strings.Join(self.prefix, ".")

		mock_call_count, _ := self.mock_call_count[artifact_name]

		// Support mocking the artifacts
		mocks, pres := self.mocks[artifact_name]
		if pres && len(mocks) > 0 {
			utils.DlvBreak()
			result := mocks[mock_call_count%len(mocks)]
			self.mock_call_count[artifact_name] = mock_call_count + 1

			a_value := reflect.Indirect(reflect.ValueOf(result))

			// It is a multi-call mock. The array represents an entire
			// call.
			if a_value.Type().Kind() == reflect.Slice {
				for i := 0; i < a_value.Len(); i++ {
					element := a_value.Index(i).Interface()
					select {
					case <-ctx.Done():
						return
					case output_chan <- element:
					}
				}

				// It is a multi-row mock of a single call - dump all
				// items into rows.
			} else {
				select {
				case <-ctx.Done():
					return
				case output_chan <- result:
				}
			}

			return
		}

		args = arg_parser.NormalizeArgs(args)

		v, pres := args.Get("source")
		if pres {
			lazy_v, ok := v.(types.LazyExpr)
			if ok {
				v = lazy_v.Reduce(ctx)
			}

			source, ok := v.(string)
			if !ok {
				scope.Log("Source must be a string")
				return
			}

			artifact_name_with_source := artifact_name + "/" + source

			artifact, pres = self.repository.Get(ctx,
				self.config_obj, artifact_name_with_source)
			if !pres {
				scope.Log("Source %v not found in artifact %v",
					source, artifact_name)
				return
			}

			artifact_name = artifact_name_with_source

		} else {

			artifact, pres = self.repository.Get(
				ctx, self.config_obj, artifact_name)
			if !pres {
				scope.Log("Artifact %v not found", artifact_name)
				return
			}
		}

		// Are preconditions required?
		var precondition bool
		precondition_any, pres := args.Get("preconditions")
		if pres {
			precondition = scope.Bool(precondition_any)
		}

		// Allow the args to specify a ** kw style args.
		kwargs_any, pres := args.Get("**")
		if pres {
			kwargs, ok := kwargs_any.(*ordereddict.Dict)
			if ok {
				args = kwargs
			}
		}

		acl_manager, ok := artifacts.GetACLManager(scope)
		if !ok {
			acl_manager = acl_managers.NullACLManager{}
		}

		launcher, err := services.GetLauncher(self.config_obj)
		if err != nil {
			scope.Log("Launcher not available")
			return
		}

		requests, err := launcher.CompileCollectorArgs(
			ctx, self.config_obj, acl_manager, self.repository,
			services.CompilerOptions{
				DisablePrecondition: !precondition,
			},
			&flows_proto.ArtifactCollectorArgs{
				Artifacts: []string{artifact_name},
			})
		if err != nil {
			scope.Log("Artifact %s invalid: %v",
				strings.Join(self.prefix, "."), err)
			return
		}

		// Wait here untill all the sources are done.
		wg := &sync.WaitGroup{}
		defer wg.Wait()

		for _, request := range requests {
			eval_request := func(
				request *actions_proto.VQLCollectorArgs,
				scope types.Scope) {

				// We create a child scope for evaluating the artifact.
				child_scope, err := self.copyScope(scope, artifact_name)
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

				// Allow the args to override the artifact defaults.
				for _, i := range args.Items() {
					if i.Key == "source" || i.Key == "preconditions" {
						continue
					}

					_, pres := env.Get(i.Key)
					if !pres {
						child_scope.Log(fmt.Sprintf(
							"Unknown parameter %s provided to artifact %v",
							i.Key, strings.Join(self.prefix, ".")))
						return
					}

					lazy_v, ok := i.Value.(types.LazyExpr)
					if ok {
						i.Value = lazy_v.Reduce(ctx)
					}
					env.Set(i.Key, i.Value)
				}

				// Add the scope args
				child_scope.AppendVars(env)

				ok, err := actions.CheckPreconditions(ctx, child_scope, request)
				if err != nil {
					child_scope.Log("While evaluating preconditions: %v", err)
					return
				}

				if !ok {
					child_scope.Log("Skipping query due to preconditions")
					return
				}

				for _, query := range request.Query {
					query_log := actions.QueryLog.AddQuery(query.VQL)
					vql, err := vfilter.Parse(query.VQL)
					if err != nil {
						child_scope.Log("Artifact %s invalid: %s",
							strings.Join(self.prefix, "."),
							err.Error())
						query_log.Close()
						return
					}

					for row := range vql.Eval(ctx, child_scope) {
						dict_row := vfilter.RowToDict(ctx, child_scope, row)
						if query.Name != "" {
							dict_row.Set("_Source", query.Name)
						}
						select {
						case <-ctx.Done():
							query_log.Close()
							return

						case output_chan <- dict_row:
						}
					}
					query_log.Close()
				}
			}

			if isEventArtifact(artifact) {
				wg.Add(1)
				go func(request *actions_proto.VQLCollectorArgs,
					scope types.Scope) {
					defer wg.Done()

					eval_request(request, scope)
				}(request, scope)
			} else {
				eval_request(request, scope)
			}
		}
	}()

	return output_chan
}

func isEventArtifact(artifact *artifacts_proto.Artifact) bool {
	switch artifact.Type {
	case "client_event", "server_event":
		return true
	}
	return false
}

// Create a mostly new scope for executing the new artifact but copy
// over some important global variables.
func (self *ArtifactRepositoryPlugin) copyScope(
	scope vfilter.Scope, my_name string) (
	vfilter.Scope, error) {
	env := ordereddict.NewDict()

	// TODO: Move most of these to the scope context as they dont
	// change with subscopes so it should be faster to get them from
	// the context.
	for _, field := range []string{
		vql_subsystem.ACL_MANAGER_VAR,
		constants.SCOPE_MOCK,
		constants.SCOPE_CONFIG,
		constants.SCOPE_THROTTLE,
		constants.SCOPE_ROOT,
		constants.SCOPE_RESPONDER,
		constants.SCOPE_REPOSITORY,
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

	// Copy critical context variables
	for _, field := range []string{
		constants.SCOPE_RESPONDER_CONTEXT,
		vql_subsystem.CACHE_VAR,
	} {
		value, pres := scope.GetContext(field)
		if pres {
			result.SetContext(field, value)
		}
	}

	return result, nil
}

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
	return nil
}

func (self _ArtifactRepositoryPluginAssociativeProtocol) Associative(
	scope vfilter.Scope, a vfilter.Any, b vfilter.Any) (vfilter.Any, bool) {

	value := _getArtifactRepositoryPlugin(a)
	if value == nil {
		return nil, false
	}

	key, pres := b.(string)
	if !pres {
		return nil, false
	}

	prefix := make([]string, 0, len(value.prefix)+1)
	for _, i := range value.prefix {
		prefix = append(prefix, i)
	}

	return &ArtifactRepositoryPlugin{
		prefix:          append(prefix, key),
		repository:      value.repository,
		config_obj:      value.config_obj,
		mocks:           value.mocks,
		mock_call_count: value.mock_call_count,
	}, true
}

func init() {
	vql_subsystem.RegisterProtocol(
		&_ArtifactRepositoryPluginAssociativeProtocol{})
}
