package common

import (
	"context"
	"regexp"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

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

		arg := &ColumnFilterArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("column_filter: %v", err)
			return
		}

		if len(arg.Include) == 0 {
			arg.Include = []string{"."}
		}

		includes := []*regexp.Regexp{}

		for _, include := range arg.Include {
			c, err := regexp.Compile(include)
			if err != nil {
				scope.Log("column_filter: %v", err)
				return
			}
			includes = append(includes, c)
		}

		excludes := []*regexp.Regexp{}

		for _, exclude := range arg.Exclude {
			c, err := regexp.Compile(exclude)
			if err != nil {
				scope.Log("column_filter: %v", err)
				return
			}
			excludes = append(excludes, c)
		}

		does_match := func(column string, checks []*regexp.Regexp) bool {
			for _, re := range checks {
				if re.MatchString(column) {
					return true
				}
			}
			return false
		}

		var column_filter map[string]bool

		// Force the in parameter to be a query.
		for item := range arg.Query.Eval(ctx, scope) {
			if column_filter == nil {
				column_filter = make(map[string]bool)
				for _, column := range scope.GetMembers(item) {
					if does_match(column, includes) &&
						!does_match(column, excludes) {
						column_filter[column] = true
					}
				}
			}

			new_row := ordereddict.NewDict()
			for _, column := range scope.GetMembers(item) {
				value, _ := scope.Associative(item, column)
				_, pres := column_filter[column]
				if pres {
					new_row.Set(column, value)
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
