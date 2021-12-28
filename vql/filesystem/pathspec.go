package filesystem

import (
	"context"
	"encoding/json"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/glob"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type PathSpecArgs struct {
	DelegateAccessor string      `vfilter:"optional,field=DelegateAccessor,doc=An accessor to use."`
	DelegatePath     string      `vfilter:"optional,field=DelegatePath,doc=A delegate to pass to the accessor."`
	Path             vfilter.Any `vfilter:"optional,field=Path,doc=A path to open."`
	Parse            string      `vfilter:"optional,field=parse,doc=Alternatively parse the pathspec from this string."`
}

type PathSpecFunction struct{}

func (self *PathSpecFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &PathSpecArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("pathspec: %s", err.Error())
		return false
	}

	if arg.Parse != "" {
		result, err := glob.PathSpecFromString(arg.Parse)
		if err != nil {
			scope.Log("pathspec: %v", err)
			return vfilter.Null{}
		}
		return result
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
	}

	switch t := path.(type) {
	case string:
		path_str = t
	default:
		serialized, err := json.Marshal(path)
		if err != nil {
			scope.Log("pathspec: %v", err)
			return vfilter.Null{}
		}

		path_str = string(serialized)
	}

	result := &glob.PathSpec{
		DelegateAccessor: arg.DelegateAccessor,
		DelegatePath:     arg.DelegatePath,
		Path:             path_str,
	}

	return result
}

func (self *PathSpecFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "pathspec",
		Doc:     "Create a structured path spec to pass to certain accessors.",
		ArgType: type_map.AddType(scope, &PathSpecArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&PathSpecFunction{})
}
