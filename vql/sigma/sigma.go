package sigma

import (
	"context"
	"strings"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/sigma-go"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"

	// For Lambda protocols
	_ "www.velocidex.com/golang/velociraptor/vql/protocols"
)

/* This provides support for direct evaluation of sigma rules. */

type SigmaPluginArgs struct {
	Rules          []string          `vfilter:"required,field=rules,doc=A list of sigma rules to compile."`
	LogSources     vfilter.Any       `vfilter:"required,field=log_sources,doc=A log source object as obtained from the sigma_log_sources() VQL function."`
	FieldMappings  *ordereddict.Dict `vfilter:"optional,field=field_mapping,doc=A dict containing a mapping between a rule field name and a VQL Lambda to get the value of the field from the event."`
	Debug          bool              `vfilter:"optional,field=debug,doc=If enabled we emit all match objects with description of what would match."`
	RuleFilter     *vfilter.Lambda   `vfilter:"optional,field=rule_filter,doc=If specified we use this callback to filter the rules for inclusion."`
	DefaultDetails *vfilter.Lambda   `vfilter:"optional,field=default_details,doc=If specified we use this callback to determine a details column if the sigma rule does not specify it."`
}

type SigmaPlugin struct{}

func (self SigmaPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "sigma", args)()

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
		for _, rules_text := range arg.Rules {
			for _, r := range strings.Split(rules_text, "\n---\n") {

				// Just ignore empty rules.
				r := strings.TrimSpace(r)
				if len(r) == 0 {
					continue
				}

				rule, err := sigma.ParseRule([]byte(r))
				if err != nil {
					// Skip the rules we can not parse
					scope.Log("sigma: Error parsing: %v in rule '%v'",
						err, utils.Elide(r, 20))
					continue
				}

				// A rule must have a title
				if rule.Title == "" {
					scope.Log("sigma: Error parsing rule '%v': no title set",
						utils.Elide(r, 20))
					continue
				}

				if arg.RuleFilter != nil &&
					!scope.Bool(arg.RuleFilter.Reduce(ctx, scope, []vfilter.Any{rule})) {
					continue
				}

				rules = append(rules, rule)
			}
		}

		// Build a new evaluation context around the rules. This binds
		// the rules to the log sources. Only the relevant log sources
		// will be evaluated - i.e. only those that have some rules
		// watching them.
		sigma_context, err := NewSigmaContext(
			ctx, scope, rules,
			arg.FieldMappings, log_sources,
			arg.DefaultDetails, arg.Debug)
		if err != nil {
			scope.Log("sigma: %v", err)
			return
		}
		defer sigma_context.Close()

		scope.Log("INFO:sigma: Loaded %v rules (from %v) into %v log sources and %v field mappings",
			sigma_context.total_rules, len(rules), len(sigma_context.runners),
			sigma_context.fieldmappings.Len())

		for row := range sigma_context.Rows(ctx, scope) {
			output_chan <- row
		}
		scope.Log("INFO:sigma: Completed with %v hits", sigma_context.GetHitCount())
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
