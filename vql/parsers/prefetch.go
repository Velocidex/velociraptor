package parsers

import (
	"context"
	"io"

	"github.com/Velocidex/ordereddict"
	prefetch "www.velocidex.com/golang/go-prefetch"
	"www.velocidex.com/golang/velociraptor/accessors"
	utils "www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

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

type _PrefetchPluginArgs struct {
	Filenames []string `vfilter:"required,field=filename,doc=A list of event log files to parse."`
	Accessor  string   `vfilter:"optional,field=accessor,doc=The accessor to use."`
}

type _PrefetchPlugin struct{}

func (self _PrefetchPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &_PrefetchPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("prefetch: %s", err.Error())
			return
		}

		for _, filename := range arg.Filenames {
			func() {
				defer utils.RecoverVQL(scope)

				err := vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
				if err != nil {
					scope.Log("prefetch: %s", err)
					return
				}

				accessor, err := accessors.GetAccessor(arg.Accessor, scope)
				if err != nil {
					scope.Log("prefetch: %v", err)
					return
				}
				fd, err := accessor.Open(filename)
				if err != nil {
					scope.Log("Unable to open file %s: %v",
						filename, err)
					return
				}
				defer fd.Close()

				reader, ok := fd.(io.ReaderAt)
				if !ok {
					scope.Log("prefetch: file is not seekable %s",
						filename)
					return
				}

				prefetch_info, err := prefetch.LoadPrefetch(reader)
				if err != nil {
					scope.Log("prefetch: Unable to parse file %s: %v",
						filename, err)
					return
				}

				select {
				case <-ctx.Done():
					return

				case output_chan <- prefetch_info:
				}
			}()
		}
	}()

	return output_chan
}

func (self _PrefetchPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "prefetch",
		Doc:     "Parses a prefetch file.",
		ArgType: type_map.AddType(scope, &_PrefetchPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&_PrefetchPlugin{})
}
