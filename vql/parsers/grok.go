package parsers

import (
	"context"
	"sort"

	"github.com/Velocidex/grok"
	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type GrokParseFunctionArgs struct {
	Grok        string      `vfilter:"required,field=grok,doc=Grok pattern."`
	Data        string      `vfilter:"required,field=data,doc=String to parse."`
	Patterns    vfilter.Any `vfilter:"optional,field=patterns,doc=Additional patterns."`
	AllCaptures bool        `vfilter:"optional,field=all_captures,doc=Extract all captures."`
}

type GrokParseFunction struct{}

func (self GrokParseFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "grok",
		Doc:     "Parse a string using a Grok expression.",
		ArgType: type_map.AddType(scope, &GrokParseFunctionArgs{}),
	}
}

func (self GrokParseFunction) Call(
	ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "grok", args)()

	arg := &GrokParseFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("grok: %v", err)
		return &vfilter.Null{}
	}

	// First retrieve the parser from the context.
	key := "__grok"
	grok_parser, ok := vql_subsystem.CacheGet(scope, key).(*grok.Grok)
	if !ok {
		grok_parser, err = grok.NewWithConfig(&grok.Config{
			NamedCapturesOnly: !arg.AllCaptures,
		})
		if err != nil {
			scope.Log("grok: %v", err)
			return &vfilter.Null{}
		}

		for _, k := range scope.GetMembers(arg.Patterns) {
			v, pres := scope.Associative(arg.Patterns, k)
			if pres {
				pattern, ok := v.(string)
				if ok {
					err = grok_parser.AddPattern(k, pattern)
					if err != nil {
						scope.Log("grok: %v", err)
						return &vfilter.Null{}
					}
				}
			}
		}

		vql_subsystem.CacheSet(scope, key, grok_parser)
	}

	// Now apply the parser on the data
	result, err := grok_parser.Parse(arg.Grok, arg.Data)
	if err != nil {
		return &vfilter.Null{}
	}

	keys := make([]string, 0, len(result))
	for k := range result {
		keys = append(keys, k)
	}

	sort.Strings(keys)
	result_dict := ordereddict.NewDict()
	for _, k := range keys {
		result_dict.Set(k, result[k])
	}
	return result_dict
}

func init() {
	vql_subsystem.RegisterFunction(&GrokParseFunction{})
}
