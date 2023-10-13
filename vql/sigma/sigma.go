package sigma

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"github.com/bradleyjkemp/sigma-go"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

/* This provides support for direct evaluation of sigma rules. */

type SigmaPluginArgs struct {
	Rules         []string          `vfilter:"required,field=rules,doc=A list of sigma rules to compile."`
	LogSources    vfilter.Any       `vfilter:"required,field=log_sources,doc=A log source object as obtained from the sigma_log_sources() VQL function."`
	FieldMappings *ordereddict.Dict `vfilter:"optional,field=field_mapping,doc=A dict containing a mapping between a rule field name and a VQL Lambda to get the value of the field from the event."`
	Debug         bool              `vfilter:"optional,field=debug,doc=If enabled we emit all match objects with description of what would match."`
	RuleFilter    *vfilter.Lambda   `vfilter:"optional,field=rule_filter,doc=If specified we use this callback to filter the rules for inclusion."`
}

type SigmaPlugin struct{}

func (self SigmaPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &SigmaPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("sigma: %v", err)
			return
		}

		log_sources, ok := arg.LogSources.(*LogSourceProvider)
		if !ok {
			scope.Log("sigma: log_sources must be a LogSourceProvider not %T. You can obtain one from the sigma_log_sources() function.", arg.LogSources)
			return
		}

		// Compile all the rules
		var rules []sigma.Rule
		for _, r := range arg.Rules {
			rule, err := sigma.ParseRule([]byte(r))
			if err != nil {
				// Skip the rules we can not parse
				scope.Log("sigma: Error parsing: %v in rule '%v'",
					err, utils.Elide(r, 20))
				continue
			}

			if arg.RuleFilter != nil &&
				!scope.Bool(arg.RuleFilter.Reduce(ctx, scope, []vfilter.Any{rule})) {
				continue
			}

			rules = append(rules, rule)
		}

		// Build a new evaluation context around the rules. This binds
		// the rules to the log sources. Only the relevant log sources
		// will be evaluated - i.e. only those that have some rules
		// watching them.
		sigma_context, err := NewSigmaContext(
			ctx, scope, rules, arg.FieldMappings, log_sources,
			arg.Debug)
		if err != nil {
			scope.Log("sigma: %v", err)
			return
		}

		scope.Log("INFO:sigma: Loaded %v rules (from %v) into %v log sources and %v field mappings",
			sigma_context.total_rules, len(rules), len(sigma_context.runners),
			len(sigma_context.fieldmappings))

		for row := range sigma_context.Rows(ctx, scope) {
			output_chan <- row
		}
	}()

	return output_chan
}

func (self SigmaPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "sigma",
		Doc:     "Evaluate sigma rules.",
		ArgType: type_map.AddType(scope, &SigmaPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&SigmaPlugin{})
}
