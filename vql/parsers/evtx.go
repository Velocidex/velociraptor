package parsers

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"time"

	"github.com/0xrawsec/golang-evtx/evtx"
	"github.com/0xrawsec/golang-utils/encoding"
	"www.velocidex.com/golang/velociraptor/glob"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

type _ParseEvtxPluginArgs struct {
	Filenames []string `vfilter:"required,field=file"`
	Accessor  string   `vfilter:"optional,field=accessor"`
}

type _ParseEvtxPlugin struct{}

func _WriteEvents(
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

	for i := uint16(0); i < header.ChunkCount; i++ {
		offsetChunk := int64(header.ChunkDataOffset) +
			int64(evtx.ChunkSize)*int64(i)

		chunk := evtx.NewChunk()
		chunk.Offset = offsetChunk
		chunk.Data = make([]byte, evtx.ChunkSize)

		file.Seek(offsetChunk, os.SEEK_SET)
		n, _ := file.Read(chunk.Data)
		if n != len(chunk.Data) || err != nil {
			return
		}

		chunk_reader := bytes.NewReader(chunk.Data)
		chunk.ParseChunkHeader(chunk_reader)

		if chunk.Header.LastEventRecID <= first_event {
			continue
		}

		chunk_reader.Seek(
			int64(chunk.Header.SizeHeader),
			os.SEEK_SET)
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
					output_chan <- item
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

				_, err = _WriteEvents(file, output_chan, 0)
				if err != nil {
					scope.Log("Error: %v", err)
					return
				}
			}()
		}
	}()

	return output_chan
}

func (self _ParseEvtxPlugin) Info(type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "parse_evtx",
		Doc:     "Parses events from an EVTX file.",
		ArgType: type_map.AddType(&_ParseEvtxPluginArgs{}),
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

		event_counts := make(map[string]int64)
		for {
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
					first_event, _ := event_counts[filename]
					last_event, err := _WriteEvents(
						file, output_chan, first_event)
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

func (self _WatchEvtxPlugin) Info(type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "watch_evtx",
		Doc: "Watch and EVTX file and stream events from it. " +
			"Note: This is an event plugin which does not complete.",
		ArgType: type_map.AddType(&_ParseEvtxPluginArgs{}),
	}
}

type _GoEvtxAssociativeProtocol struct{}

func (self _GoEvtxAssociativeProtocol) Applicable(
	a vfilter.Any, b vfilter.Any) bool {
	_, b_ok := b.(string)
	if !b_ok {
		return false
	}

	switch a.(type) {
	case evtx.GoEvtxMap, *evtx.GoEvtxMap:
		return true
	default:
		return false
	}
}

func (self _GoEvtxAssociativeProtocol) Associative(
	scope *vfilter.Scope, a vfilter.Any, b vfilter.Any) (
	vfilter.Any, bool) {
	var a_map *evtx.GoEvtxMap

	switch t := a.(type) {
	case evtx.GoEvtxMap:
		a_map = &t
	case *evtx.GoEvtxMap:
		a_map = t
	default:
		return vfilter.Null{}, false
	}

	key, key_ok := b.(string)
	if key_ok {
		result, pres := (*a_map)[key]
		if pres {
			return result, true
		}

		// Try a case insensitive match.
		key = strings.ToLower(key)
		for k, v := range *a_map {
			if strings.ToLower(k) == key {
				return v, true
			}
		}
	}
	return vfilter.Null{}, false
}

func (self _GoEvtxAssociativeProtocol) GetMembers(
	scope *vfilter.Scope, a vfilter.Any) []string {
	result := []string{}
	a_map, ok := a.(evtx.GoEvtxMap)
	if ok {
		for k, _ := range a_map {
			result = append(result, k)
		}
	}

	return result
}

func init() {
	vql_subsystem.RegisterPlugin(&_ParseEvtxPlugin{})
	vql_subsystem.RegisterPlugin(&_WatchEvtxPlugin{})
	vql_subsystem.RegisterProtocol(&_GoEvtxAssociativeProtocol{})
}
