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
package event_logs

import (
	"context"
	"os"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/evtx"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/file"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type _CarveEvtxPluginArgs struct {
	Filename    *accessors.OSPath `vfilter:"required,field=filename,doc=The file or device to carve (e.g. \\\\.\\C: with the raw_file accessor, or C:/pagefile.sys with the ntfs accessor)."`
	Accessor    string            `vfilter:"optional,field=accessor,doc=The accessor to use."`
	Database    string            `vfilter:"optional,field=messagedb,doc=A Message database from https://github.com/Velocidex/evtx-data."`
	StartOffset int64             `vfilter:"optional,field=start_offset,doc=Start carving at this offset (default 0)."`
	EndOffset   int64             `vfilter:"optional,field=end_offset,doc=Stop carving at this offset (default end of file)."`
	Output      string            `vfilter:"optional,field=output,doc=If set, also write the recovered chunks to a new EVTX file at this path so they can be opened with Event Viewer. Each chunk is written once."`
}

type _CarveEvtxPlugin struct{}

func (self _CarveEvtxPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "carve_evtx", args)()
		defer utils.RecoverVQL(scope)

		arg := &_CarveEvtxPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("carve_evtx: %v", err)
			return
		}

		resolver, err := getMessageResolver(scope, arg.Database)
		if err != nil {
			scope.Log("carve_evtx: %v", err)
			return
		}

		accessor, err := accessors.GetAccessor(arg.Accessor, scope)
		if err != nil {
			scope.Log("carve_evtx: %v", err)
			return
		}

		fd, err := accessor.OpenWithOSPath(arg.Filename)
		if err != nil {
			scope.Log("carve_evtx: Unable to open %v: %v", arg.Filename, err)
			return
		}
		defer fd.Close()

		var writer *evtxWriter
		if arg.Output != "" {
			out_fd, err := openOutputFile(scope, arg.Output)
			if err != nil {
				scope.Log("carve_evtx: %v", err)
				return
			}
			defer out_fd.Close()

			writer, err = newEvtxWriter(out_fd)
			if err != nil {
				scope.Log("carve_evtx: Unable to write %v: %v", arg.Output, err)
				return
			}

			defer func() {
				err := writer.Close()
				if err != nil {
					scope.Log("carve_evtx: Unable to write %v: %v", arg.Output, err)
					return
				}
				scope.Log("carve_evtx: Wrote %v unique chunks to %v",
					writer.Chunks(), arg.Output)
			}()
		}

		scope.Log("carve_evtx: Carving %v for EVTX chunks", arg.Filename)

		stats := &carveStats{}
		var records_emitted int64

		sub_ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		reader := utils.MakeReaderAtter(fd)
		for chunk := range carveChunks(
			sub_ctx, reader, arg.StartOffset, arg.EndOffset, stats) {
			scope.ChargeOp()

			if writer != nil && includeInOutput(chunk) {
				_, err := writer.WriteChunk(chunk)
				if err != nil {
					scope.Log("carve_evtx: Unable to write %v: %v", arg.Output, err)
					return
				}
			}

			records, err := parseCarvedChunk(chunk)
			if err != nil && len(records) == 0 {
				scope.Log("carve_evtx: chunk at offset %#x: %v", chunk.Offset, err)
				continue
			}

			for _, record := range records {
				row := makeCarvedRow(chunk, record, resolver)
				if row == nil {
					continue
				}
				records_emitted++

				select {
				case <-ctx.Done():
					return
				case output_chan <- row:
				}
			}
		}

		if stats.StopError != nil {
			scope.Log("carve_evtx: Stopped at offset %#x: %v",
				arg.StartOffset+stats.BytesScanned, stats.StopError)
		}

		scope.Log("carve_evtx: Scanned %v bytes, found %v chunk signatures, %v plausible chunks, %v records",
			stats.BytesScanned, stats.Candidates, stats.Chunks, records_emitted)
	}()

	return output_chan
}

// Chunks written to the output file.
func includeInOutput(chunk *carvedChunk) bool {
	return true
}

// Create the output file after checking that the query may write
// there.
func openOutputFile(scope vfilter.Scope, path string) (*os.File, error) {
	err := vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_WRITE)
	if err != nil {
		return nil, err
	}

	err = file.CheckPath(path)
	if err != nil {
		return nil, err
	}

	return os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
}

// Emit the same event structure as parse_evtx() with details about
// where the record was found and how trustworthy its chunk is.
func makeCarvedRow(chunk *carvedChunk, record *evtx.EventRecord,
	resolver evtx.MessageResolver) *ordereddict.Dict {

	event_map, ok := record.Event.(*ordereddict.Dict)
	if !ok {
		return nil
	}

	event, pres := ordereddict.GetMap(event_map, "Event")
	if !pres {
		return nil
	}

	if resolver != nil {
		event.Set("Message", evtx.ExpandMessage(event, resolver))
	}

	return event.
		Set("ChunkOffset", chunk.Offset).
		Set("ChunkHeaderValid", chunk.HeaderChecksumValid).
		Set("ChunkDataValid", chunk.DataChecksumValid).
		Set("ChunkTruncated", chunk.Truncated).
		Set("ChunkCompressed", chunk.Compressed)
}

func (self _CarveEvtxPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "carve_evtx",
		Doc: "Carve EVTX chunks from a file or device (e.g. unallocated space, " +
			"the pagefile or a memory image) and parse their event records.",
		ArgType: type_map.AddType(scope, &_CarveEvtxPluginArgs{}),
		// Writing the output file checks FILESYSTEM_WRITE when it is used.
		Metadata: vql_subsystem.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
		Version:  1,
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&_CarveEvtxPlugin{})
}
