/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2022 Rapid7 Inc.

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
package event_logs

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/evtx"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type _ParseEvtxPluginArgs struct {
	Filenames []*accessors.OSPath `vfilter:"required,field=filename,doc=A list of event log files to parse."`
	Accessor  string              `vfilter:"optional,field=accessor,doc=The accessor to use."`
	Database  string              `vfilter:"optional,field=messagedb,doc=A Message database from https://github.com/Velocidex/evtx-data."`
}

type _ParseEvtxPlugin struct{}

func (self _ParseEvtxPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &_ParseEvtxPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("parse_evtx: %s", err.Error())
			return
		}

		var resolver evtx.MessageResolver
		if arg.Database != "" {
			resolver, err = evtx.NewDBResolver(arg.Database)
		} else {
			// If the plugin did not specify a database, use the local
			// resolver - On windows this will search DLLs for the messages.
			resolver, err = evtx.GetNativeResolver()
		}

		if err != nil {
			scope.Log("parse_evtx: %s", err.Error())
			return
		}

		// Close the db when we are done.
		vql_subsystem.GetRootScope(scope).AddDestructor(resolver.Close)

		for _, filename := range arg.Filenames {
			func() {
				defer utils.RecoverVQL(scope)

				err := vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
				if err != nil {
					scope.Log("parse_evtx: %s", err)
					return
				}

				accessor, err := accessors.GetAccessor(arg.Accessor, scope)
				if err != nil {
					scope.Log("parse_evtx: %v", err)
					return
				}
				fd, err := accessor.OpenWithOSPath(filename)
				if err != nil {
					scope.Log("Unable to open file %s: %v",
						filename, err)
					return
				}
				defer fd.Close()

				chunks, err := evtx.GetChunks(fd)
				if err != nil {
					scope.Log("Unable to parse file %s: %v",
						filename, err)
					return
				}

				for _, chunk := range chunks {
					records, _ := chunk.Parse(0)
					for _, i := range records {
						event_map, ok := i.Event.(*ordereddict.Dict)
						if !ok {
							continue
						}
						event, pres := ordereddict.GetMap(event_map, "Event")
						if !pres {
							continue
						}

						if resolver != nil {
							event.Set("Message", evtx.ExpandMessage(event, resolver))
						}

						select {
						case <-ctx.Done():
							return

						case output_chan <- event:
						}
					}
				}

			}()
		}
	}()

	return output_chan
}

func (self _ParseEvtxPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "parse_evtx",
		Doc:      "Parses events from an EVTX file.",
		ArgType:  type_map.AddType(scope, &_ParseEvtxPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

type _WatchEvtxPlugin struct{}

func (self _WatchEvtxPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		// Do not close output_chan - The event log service
		// owns it and it will be closed by it.
		arg := &_ParseEvtxPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("watch_evtx: %s", err.Error())
			return
		}

		err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
		if err != nil {
			scope.Log("watch_evtx: %s", err)
			return
		}

		// https://go101.org/article/channel-closing.html We
		// must not close the channel on the receiving side,
		// just let the receiver cancel then the context is
		// done. Note that event_channel is not explicitly
		// closed at all since all its senders will terminate
		// when the context is done.
		event_channel := make(chan vfilter.Row)

		// Register the output channel as a listener to the
		// global event.
		for _, filename := range arg.Filenames {
			cancel := GlobalEventLogService.Register(
				filename, arg.Accessor,
				ctx, scope, event_channel)
			defer cancel()
		}

		// Wait until the query is complete.
		for event := range event_channel {
			select {
			case <-ctx.Done():
				return

			case output_chan <- event:
			}
		}
	}()

	return output_chan
}

func (self _WatchEvtxPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "watch_evtx",
		Doc:      "Watch an EVTX file and stream events from it. ",
		ArgType:  type_map.AddType(scope, &_ParseEvtxPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&_ParseEvtxPlugin{})
	vql_subsystem.RegisterPlugin(&_WatchEvtxPlugin{})
}
