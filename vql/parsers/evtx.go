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
	"bytes"
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/0xrawsec/golang-evtx/evtx"
	"github.com/0xrawsec/golang-utils/encoding"
	"www.velocidex.com/golang/velociraptor/glob"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

// Get rid of the complex goevtx object and just return a
// map[string]interface{}
func Normalize(event *evtx.GoEvtxMap) (map[string]interface{}, error) {
	encoded, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}

	result := make(map[string]interface{})
	err = json.Unmarshal(encoded, &result)
	return result, err
}

type _ParseEvtxPluginArgs struct {
	Filenames []string `vfilter:"required,field=filename"`
	Accessor  string   `vfilter:"optional,field=accessor"`
}

type _ParseEvtxPlugin struct{}

func _WriteEvents(
	scope *vfilter.Scope,
	file io.ReadSeeker,
	output_chan chan vfilter.Row,
	first_event int64) (
	last_event int64, err error) {
	header := &evtx.FileHeader{}
	err = encoding.Unmarshal(file, header, evtx.Endianness)
	if err != nil {
		return
	}

	last_event = first_event

	for i := int64(0); ; i++ {
		offsetChunk := int64(header.ChunkDataOffset) +
			int64(evtx.ChunkSize)*i

		chunk := evtx.NewChunk()
		chunk.Offset = offsetChunk
		chunk.Data = make([]byte, evtx.ChunkSize)

		offset, err := file.Seek(offsetChunk, io.SeekStart)
		if err != nil {
			continue
		}
		if offset != offsetChunk || err != nil {
			return last_event, nil
		}

		n, err := io.ReadAtLeast(file, chunk.Data, len(chunk.Data))
		if n != len(chunk.Data) || err != nil {
			return last_event, nil
		}

		chunk_reader := bytes.NewReader(chunk.Data)
		chunk.ParseChunkHeader(chunk_reader)

		if chunk.Header.LastEventRecID <= first_event {
			continue
		}

		chunk_reader.Seek(
			int64(chunk.Header.SizeHeader),
			io.SeekStart)
		chunk.ParseStringTable(chunk_reader)
		err = chunk.ParseTemplateTable(chunk_reader)
		if err != nil {
			continue
		}

		err = chunk.ParseEventOffsets(chunk_reader)
		if err != nil {
			continue
		}

		for _, event_offset := range chunk.EventOffsets {
			event := chunk.ParseEvent(int64(event_offset))
			item, err := event.GoEvtxMap(&chunk)
			if err == nil {
				if item.EventRecordID() > last_event {
					event, err := Normalize(item)
					if err == nil {
						event_details, pres := event["Event"]
						if pres && output_chan != nil {
							output_chan <- event_details
						}
					}
					last_event = item.EventRecordID()
				}
			}
		}
	}

	return
}

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
				file, err := accessor.Open(filename)
				if err != nil {
					scope.Log("Unable to open file %s: %v",
						filename, err)
					return
				}
				defer file.Close()

				_, err = _WriteEvents(scope, file, output_chan, 0)
				if err != nil {
					scope.Log("Error: %v", err)
					return
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
		event_counts := make(map[string]int64)

		// Parse the files once to get the last event
		// id. After this we will watch for new events added
		// to the file.
		for _, filename := range arg.Filenames {
			func() {
				file, err := accessor.Open(filename)
				if err != nil {
					return
				}
				defer file.Close()

				last_event, err := _WriteEvents(
					scope, file, nil, 0)

				if err == nil {
					event_counts[filename] = last_event
				}
			}()
		}

		for {
			for _, filename := range arg.Filenames {
				func() {
					file, err := accessor.Open(filename)
					if err != nil {
						scope.Log("Unable to open file %s: %v",
							filename, err)
						return
					}
					defer file.Close()
					last_event, err := _WriteEvents(
						scope, file, output_chan,
						event_counts[filename])
					if err != nil {
						scope.Log("Error: %v", err)
						return
					}

					event_counts[filename] = last_event
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
