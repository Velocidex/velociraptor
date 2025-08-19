package common

import (
	"context"
	"regexp"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type matcher struct {
	includes []*regexp.Regexp
	excludes []*regexp.Regexp

	cache map[string]bool
}

func matchAny(needle string, filters []*regexp.Regexp) bool {
	for _, f := range filters {
		if f.MatchString(needle) {
			return true
		}
	}
	return false
}

func (self *matcher) ShouldInclude(column string) bool {
	res, pres := self.cache[column]
	if pres {
		return res
	}

	// Column has to appear in the include set if an include set is
	// specified.
	if len(self.includes) > 0 &&
		!matchAny(column, self.includes) {
		self.cache[column] = false
		return false
	}

	// If the column is in the exclude set, then we exclude it.
	if len(self.excludes) > 0 &&
		matchAny(column, self.excludes) {
		self.cache[column] = false
		return false
	}

	self.cache[column] = true
	return true
}

func newMatcher(includes []string, excludes []string) (*matcher, error) {
	self := &matcher{
		cache: make(map[string]bool),
	}
	for _, include := range includes {
		c, err := regexp.Compile(include)
		if err != nil {
			return nil, err
		}
		self.includes = append(self.includes, c)
	}

	for _, exclude := range excludes {
		c, err := regexp.Compile(exclude)
		if err != nil {
			return nil, err
		}
		self.excludes = append(self.excludes, c)
	}

	return self, nil
}

type ColumnFilterArgs struct {
	Query   vfilter.StoredQuery `vfilter:"required,field=query,doc=This query will be run to produce the columns."`
	Exclude []string            `vfilter:"optional,field=exclude,doc=One of more regular expressions that will exclude columns."`
	Include []string            `vfilter:"optional,field=include,doc=One of more regular expressions that will include columns."`
}

type ColumnFilter struct{}

func (self ColumnFilter) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "column_filter", args)()

		arg := &ColumnFilterArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("column_filter: %v", err)
			return
		}

		column_matcher, err := newMatcher(arg.Include, arg.Exclude)
		if err != nil {
			scope.Log("column_filter: %v", err)
			return
		}

		// Force the in parameter to be a query.
		for item := range arg.Query.Eval(ctx, scope) {
			new_row := ordereddict.NewDict()
			for _, column := range scope.GetMembers(item) {
				if column_matcher.ShouldInclude(column) {
					value, pres := scope.Associative(item, column)
					if pres {
						new_row.Set(column, value)
					}
				}
			}

			select {
			case <-ctx.Done():
				return

			case output_chan <- new_row:
			}
		}

	}()

	return output_chan
}

func (self ColumnFilter) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "column_filter",
		Doc:     "Select columns from another query using regex.",
		ArgType: type_map.AddType(scope, &ColumnFilterArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&ColumnFilter{})
}
