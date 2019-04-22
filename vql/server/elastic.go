// +build elastic

// This module addes a dependency on elsatic.v5 which turns out to be
// huge! Disabling for now:

// $ goweight ./bin/
// 7.8 MB www.velocidex.com/golang/velociraptor/vendor/gopkg.in/olivere/elastic.v5

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

/* Plugin Elastic.


 */
package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	elastic "gopkg.in/olivere/elastic.v5"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

type _ElasticPluginArgs struct {
	Query   vfilter.StoredQuery `vfilter:"required,field=query,doc=Source for rows to upload."`
	Threads int64               `vfilter:"optional,field=threads,doc=How many threads to use."`
	Index   string              `vfilter:"required,field=index,doc=The name of the index to upload to."`
	Type    string              `vfilter:"required,field=type,doc=The name of the type to use."`
}

type _ElasticPlugin struct{}

func (self _ElasticPlugin) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &_ElasticPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("elastic: %v", err)
			return
		}

		if arg.Threads == 0 {
			arg.Threads = 2
		}

		wg := sync.WaitGroup{}
		row_chan := arg.Query.Eval(ctx, scope)
		for i := 0; i < int(arg.Threads); i++ {
			wg.Add(1)

			go func() {
				defer wg.Done()

				client, err := elastic.NewClient()
				if err != nil {
					scope.Log("elastic: %v", err)
					return
				}

				id := time.Now().UnixNano() + int64(i)*10000000

				for row := range row_chan {
					id = id + 1
					_, err := client.Index().Index(arg.Index).
						Type(arg.Type).
						Id(fmt.Sprintf("%d", id)).
						BodyJson(vql_subsystem.RowToDict(scope, row)).
						Do(ctx)
					if err != nil {
						scope.Log("elastic: %v", err)
					}
				}
			}()
		}

		wg.Wait()
	}()
	return output_chan
}

func (self _ElasticPlugin) Info(
	scope *vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "elastic_upload",
		Doc:  "Upload rows to elastic.",

		ArgType: type_map.AddType(scope, &_ElasticPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&_ElasticPlugin{})
}
