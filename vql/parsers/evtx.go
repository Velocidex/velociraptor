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
package parsers

import (
	"context"
	"time"

	"www.velocidex.com/golang/evtx"
	"www.velocidex.com/golang/velociraptor/glob"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

type _ParseEvtxPluginArgs struct {
	Filenames []string `vfilter:"required,field=filename"`
	Accessor  string   `vfilter:"optional,field=accessor"`
}

type _ParseEvtxPlugin struct{}

func (self _ParseEvtxPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &_ParseEvtxPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("parse_evtx: %s", err.Error())
			return
		}

		for _, filename := range arg.Filenames {
			func() {
				accessor := glob.GetAccessor(arg.Accessor, ctx)
				fd, err := accessor.Open(filename)
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
						event_map, ok := i.Event.(map[string]interface{})
						if ok {
							output_chan <- event_map["Event"]
						}
					}
				}

			}()
		}
	}()

	return output_chan
}

func (self _ParseEvtxPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "parse_evtx",
		Doc:     "Parses events from an EVTX file.",
		ArgType: type_map.AddType(scope, &_ParseEvtxPluginArgs{}),
	}
}

type _WatchEvtxPlugin struct{}

func (self _WatchEvtxPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &_ParseEvtxPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("watch_evtx: %s", err.Error())
			return
		}

		accessor := glob.GetAccessor(arg.Accessor, ctx)
		event_counts := make(map[string]uint64)

		// Parse the files once to get the last event
		// id. After this we will watch for new events added
		// to the file.
		for _, filename := range arg.Filenames {
			func() {
				fd, err := accessor.Open(filename)
				if err != nil {
					return
				}
				defer fd.Close()

				chunks, err := evtx.GetChunks(fd)
				if err != nil {
					return
				}
				last_event := uint64(0)
				for _, c := range chunks {
					if c.Header.LastEventRecID > last_event {
						last_event = c.Header.LastEventRecID
					}
				}
				event_counts[filename] = last_event
			}()
		}

		for {
			for _, filename := range arg.Filenames {
				func() {
					fd, err := accessor.Open(filename)
					if err != nil {
						scope.Log("Unable to open file %s: %v",
							filename, err)
						return
					}
					defer fd.Close()

					last_event := event_counts[filename]
					chunks, err := evtx.GetChunks(fd)
					if err != nil {
						return
					}

					new_last_event := last_event
					for _, c := range chunks {
						if c.Header.LastEventRecID <= last_event {
							continue
						}

						records, _ := c.Parse(int(last_event))
						for _, record := range records {
							if record.Header.RecordID > new_last_event {
								new_last_event = record.Header.RecordID
							}
							event_map, ok := record.Event.(map[string]interface{})
							if ok {
								output_chan <- event_map["Event"]
							}
						}
					}

					event_counts[filename] = new_last_event
				}()
			}

			time.Sleep(10 * time.Second)
		}

	}()

	return output_chan
}

func (self _WatchEvtxPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "watch_evtx",
		Doc: "Watch an EVTX file and stream events from it. " +
			"Note: This is an event plugin which does not complete.",
		ArgType: type_map.AddType(scope, &_ParseEvtxPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&_ParseEvtxPlugin{})
	vql_subsystem.RegisterPlugin(&_WatchEvtxPlugin{})
}
