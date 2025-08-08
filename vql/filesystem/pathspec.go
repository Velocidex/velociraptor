package filesystem

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type PathSpecArgs struct {
	DelegateAccessor string           `vfilter:"optional,field=DelegateAccessor,doc=An accessor to use."`
	DelegatePath     string           `vfilter:"optional,field=DelegatePath,doc=A delegate to pass to the accessor."`
	Delegate         vfilter.LazyExpr `vfilter:"optional,field=Delegate,doc=A delegate to pass to the accessor (must be another pathspec)."`
	Path             vfilter.Any      `vfilter:"optional,field=Path,doc=A path to open."`
	Parse            string           `vfilter:"optional,field=parse,doc=Alternatively parse the pathspec from this string."`
	Type             string           `vfilter:"optional,field=path_type,doc=Type of path this is (windows,linux,registry,ntfs)."`
	Accessor         string           `vfilter:"optional,field=accessor,doc=The accessor to use to parse the path with"`
}

type PathSpecFunction struct{}

func (self PathSpecFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "pathspec", args)()

	arg := &PathSpecArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("pathspec: %v", err)
		return false
	}

	if arg.Parse != "" {
		os_path, err := accessors.ParsePath(arg.Parse, arg.Type)
		if err != nil {
			scope.Log("pathspec: %v", err)
			return false
		}

		return os_path
	}

	// The path can be a more complex type
	var path vfilter.Any
	var path_str string

	switch t := arg.Path.(type) {
	case vfilter.StoredQuery:
		path_slice := []vfilter.Any{}
		for row := range t.Eval(ctx, scope) {
			path_slice = append(path_slice, row)
		}
		path = path_slice

	case vfilter.LazyExpr:
		path = t.Reduce(ctx)

	default:
		path = arg.Path
	}

	switch t := path.(type) {
	case string:
		path_str = t

	default:
		if !utils.IsNil(path) {
			serialized, err := json.Marshal(path)
			if err != nil {
				scope.Log("pathspec: %v", err)
				return vfilter.Null{}
			}

			path_str = string(serialized)
			p := &accessors.PathSpec{
				DelegateAccessor: arg.DelegateAccessor,
				DelegatePath:     arg.DelegatePath,
				Path:             path_str,
			}

			result := accessors.MustNewPathspecOSPath(p.String())
			return result
		}
	}

	if arg.Accessor != "" {
		accessor, err := accessors.GetAccessor(arg.Accessor, scope)
		if err != nil {
			scope.Log("pathspec: %v", err)
			return false
		}
		result, err := accessor.ParsePath(path_str)
		if err == nil {
			return result
		}
	}

	result, err := accessors.ParsePath(path_str, arg.Type)
	if err != nil {
		scope.Log("pathspec: %v", err)
		return vfilter.Null{}
	}

	ps := &accessors.PathSpec{
		DelegateAccessor: arg.DelegateAccessor,
		DelegatePath:     arg.DelegatePath,
		Path:             path_str,
	}

	if arg.Delegate != nil {
		delegate := arg.Delegate.Reduce(ctx)
		if !utils.IsNil(delegate) {
			switch t := delegate.(type) {
			case *accessors.PathSpec:
				ps.Delegate = t

			case *accessors.OSPath:
				ps.Delegate = t.PathSpec()

			default:
				scope.Log("pathspec: delegate %v is of type %T but should be a pathspec",
					delegate, delegate)
				return vfilter.Null{}
			}

		}
	}

	err = result.SetPathSpec(ps)
	if err != nil {
		scope.Log("pathspec: %v", err)
		return vfilter.Null{}
	}
	return result
}

func (self PathSpecFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "pathspec",
		Doc:     "Create a structured path spec to pass to certain accessors.",
		ArgType: type_map.AddType(scope, &PathSpecArgs{}),
		Version: 1,
	}
}

func init() {
	vql_subsystem.RegisterFunction(&PathSpecFunction{})
}
