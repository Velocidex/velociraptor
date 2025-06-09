/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2025 Rapid7 Inc.

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

/*
Plugin diff.

The diff plugin runs a query periodically and stores the result set in
memory. The next time the query is run, any rows not present in the
old query are emitted with the term "added" and any rows present in
the old query and not present in the new query are termed "removed".

In order to detect if a row is present or not we use a key which is a
single column name.

Here is an example for a query which monitors a directory for the
presence or removal of text files:

	 SELECT * FROM diff(
		query={
		  SELECT FullPath, Size FROM glob(globs='/etc/*.txt')
		},
		period=10,
		key='FullPath')

The key must be a string. You can create the key using the format()
VQL function where you can combine several columns to create a unique
key. For example watching for new files or modified files can be achieved by:

SELECT format(format="%v@%v", args=[FullPath, Mtime.Unix]) as Key, ....
*/
package common

import (
	"context"
	"fmt"
	"time"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type _DiffCache struct {
	rows         map[string][]*ordereddict.Dict
	stored_query vfilter.StoredQuery
	key          string
	done         chan bool
}

func (self *_DiffCache) Eval(ctx context.Context, scope vfilter.Scope) []vfilter.Row {
	result := []vfilter.Row{}
	old_rows_map := self.rows
	self.rows = make(map[string][]*ordereddict.Dict)

	subscope := scope.Copy()
	defer subscope.Close()

	row_chan := self.stored_query.Eval(ctx, subscope)
	added_keys := []string{}

check_row:
	for {
		select {
		case <-self.done: // Scope is destroyed, cancel.
			return nil

		case row, ok := <-row_chan:
			if !ok {
				break check_row
			}

			new_key_any, pres := scope.Associative(row, self.key)
			if !pres {
				continue
			}
			new_key := fmt.Sprintf("%v", new_key_any)

			dict_row := vfilter.RowToDict(ctx, subscope, row)

			self.rows[new_key] = append(self.rows[new_key], dict_row)

			// If this is the first time we ran we need to not
			// emit anything because this is the baseline.
			if old_rows_map == nil {
				continue
			}

			_, pres = old_rows_map[new_key]
			if !pres {
				// These are new rows added.
				result = append(
					result,
					dict_row.Set("Diff", "added"))
			} else {
				// Same rows exist in old
				// query. Remove them from the map.
				added_keys = append(added_keys, new_key)
			}
		}
	}

	// Remove the added keys from the old map, what is left is the
	// rows that were deleted in this query.
	for _, added_key := range added_keys {
		delete(old_rows_map, added_key)
	}

	// Now emit the deleted rows - these are just the keys left over.
	for _, rows := range old_rows_map {
		for _, row := range rows {
			result = append(result, row.Set("Diff", "removed"))
		}
	}

	return result
}

func NewDiffCache(
	ctx context.Context,
	scope vfilter.Scope,
	period time.Duration,
	key string,
	stored_query vfilter.StoredQuery) *_DiffCache {
	result := &_DiffCache{
		key:          key,
		stored_query: stored_query,
		done:         make(chan bool),
	}

	err := scope.AddDestructor(func() {
		close(result.done)
	})
	if err != nil {
		close(result.done)
		scope.Log("NewDiffCache AddDestructor: %s", err)
	}

	return result
}

type _DiffPluginArgs struct {
	Query  vfilter.StoredQuery `vfilter:"required,field=query,doc=Source for cached rows."`
	Key    string              `vfilter:"required,field=key,doc=The column to use as key."`
	Period int64               `vfilter:"optional,field=period,doc=Number of seconds between evaluation of the query."`
}

type _DiffPlugin struct{}

func (self _DiffPlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "diff", args)()

		arg := &_DiffPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("diff: %v", err)
			return
		}

		if arg.Period == 0 {
			arg.Period = 60
		}

		// Get a unique key for this query.
		diff_cache := NewDiffCache(
			ctx, scope,
			time.Duration(arg.Period)*time.Second,
			arg.Key,
			arg.Query)

		for {
			scope.Log("diff: Running query")
			for _, row := range diff_cache.Eval(ctx, scope) {
				select {
				case <-ctx.Done():
					return

				case output_chan <- row:
				}
			}

			select {
			case <-ctx.Done():
				return

			case <-time.After(time.Duration(arg.Period) * time.Second):
			}
		}

	}()
	return output_chan
}

func (self _DiffPlugin) Info(
	scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "diff",
		Doc:  "Executes 'query' periodically and emit differences from the last query.",

		ArgType: type_map.AddType(scope, &_DiffPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&_DiffPlugin{})
}
