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

	"github.com/Velocidex/ordereddict"
	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/paths"
	utils "www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/windows/filesystems/readers"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type NTFSFunctionArgs struct {
	Device    string `vfilter:"required,field=device,doc=The device file to open. This may be a full path - we will figure out the device automatically."`
	Inode     string `vfilter:"optional,field=inode,doc=The MFT entry to parse in inode notation (5-144-1)."`
	MFT       int64  `vfilter:"optional,field=mft,doc=The MFT entry to parse."`
	MFTOffset int64  `vfilter:"optional,field=mft_offset,doc=The offset to the MFT entry to parse."`
}

type NTFSModel struct {
	*ntfs.NTFSFileInformation

	Device string
}

type NTFSFunction struct{}

func (self NTFSFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "parse_ntfs",
		Doc:     "Parse an NTFS image file.",
		ArgType: type_map.AddType(scope, &NTFSFunctionArgs{}),
	}
}

func (self NTFSFunction) Call(
	ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer utils.RecoverVQL(scope)

	arg := &NTFSFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("parse_ntfs: %v", err)
		return &vfilter.Null{}
	}

	if arg.Inode != "" {
		mft_idx, _, _, err := ntfs.ParseMFTId(arg.Inode)
		if err != nil {
			scope.Log("parse_ntfs: %v", err)
			return &vfilter.Null{}
		}
		arg.MFT = mft_idx
	}

	device, _, err := paths.GetDeviceAndSubpath(arg.Device)
	if err != nil {
		scope.Log("parse_ntfs: %v", err)
		return &vfilter.Null{}
	}

	ntfs_ctx, err := readers.GetNTFSContext(scope, device, "file")
	if err != nil {
		scope.Log("parse_ntfs: GetNTFSContext %v", err)
		return &vfilter.Null{}
	}

	if ntfs_ctx == nil || ntfs_ctx.Boot == nil {
		scope.Log("parse_ntfs: invalid context")
		return &vfilter.Null{}
	}

	if arg.MFTOffset > 0 {
		arg.MFT = arg.MFTOffset / ntfs_ctx.Boot.ClusterSize()
	}

	mft_entry, err := ntfs_ctx.GetMFT(arg.MFT)
	if err != nil {
		scope.Log("parse_ntfs: GetMFT %v", err)
		return &vfilter.Null{}
	}

	result, err := ntfs.ModelMFTEntry(ntfs_ctx, mft_entry)
	if err != nil {
		scope.Log("parse_ntfs: ModelMFTEntry %v", err)
		return &vfilter.Null{}
	}

	return &NTFSModel{NTFSFileInformation: result, Device: device}
}

type MFTScanPluginArgs struct {
	Filename string `vfilter:"required,field=filename,doc=A list of event log files to parse."`
	Accessor string `vfilter:"optional,field=accessor,doc=The accessor to use."`
}

type MFTScanPlugin struct{}

func (self MFTScanPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer utils.RecoverVQL(scope)

		arg := &MFTScanPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("parse_mft: %v", err)
			return
		}

		err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
		if err != nil {
			scope.Log("parse_mft: %s", err)
			return
		}

		accessor, err := glob.GetAccessor(arg.Accessor, scope)
		if err != nil {
			scope.Log("parse_mft: %v", err)
			return
		}
		fd, err := accessor.Open(arg.Filename)
		if err != nil {
			scope.Log("parse_mft: Unable to open file %s: %v",
				arg.Filename, err)
			return
		}
		defer fd.Close()

		reader, err := ntfs.NewPagedReader(
			utils.ReaderAtter{Reader: fd}, 1024, 10000)
		if err != nil {
			scope.Log("parse_mft: Unable to open file %s: %v",
				arg.Filename, err)
			return
		}

		st, err := fd.Stat()
		if err != nil {
			scope.Log("parse_mft: Unable to open file %s: %v",
				arg.Filename, err)
			return
		}

		for item := range ntfs.ParseMFTFile(
			ctx, reader, st.Size(), 0x1000, 0x400) {
			select {
			case <-ctx.Done():
				return

			case output_chan <- item:
			}
		}
	}()

	return output_chan
}

func (self MFTScanPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "parse_mft",
		Doc:     "Scan the $MFT from an NTFS volume.",
		ArgType: type_map.AddType(scope, &MFTScanPluginArgs{}),
	}
}

type NTFSI30ScanPlugin struct{}

func (self NTFSI30ScanPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer utils.RecoverVQL(scope)

		arg := &NTFSFunctionArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("parse_ntfs_i30: %v", err)
			return
		}

		if arg.Inode != "" {
			mft_idx, _, _, err := ntfs.ParseMFTId(arg.Inode)
			if err != nil {
				scope.Log("parse_ntfs_i30: %v", err)
				return
			}
			arg.MFT = mft_idx
		}

		device, _, err := paths.GetDeviceAndSubpath(arg.Device)
		if err != nil {
			scope.Log("parse_ntfs_i30: %v", err)
			return
		}

		ntfs_ctx, err := readers.GetNTFSContext(scope, device, "file")
		if err != nil {
			scope.Log("parse_ntfs_i30: %v", err)
			return
		}

		if arg.MFTOffset > 0 {
			arg.MFT = arg.MFTOffset / ntfs_ctx.Boot.ClusterSize()
		}

		mft_entry, err := ntfs_ctx.GetMFT(arg.MFT)
		if err != nil {
			scope.Log("parse_ntfs_i30: %v", err)
			return
		}

		for _, fileinfo := range ntfs.ExtractI30List(ntfs_ctx, mft_entry) {
			select {
			case <-ctx.Done():
				return

			case output_chan <- fileinfo:
			}
		}
	}()

	return output_chan
}

func (self NTFSI30ScanPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "parse_ntfs_i30",
		Doc:     "Scan the $I30 stream from an NTFS MFT entry.",
		ArgType: type_map.AddType(scope, &NTFSFunctionArgs{}),
	}
}

type NTFSRangesPlugin struct{}

func (self NTFSRangesPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer utils.RecoverVQL(scope)

		arg := &NTFSFunctionArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("parse_ntfs_ranges: %v", err)
			return
		}

		attr_type := int64(0)
		attr_id := int64(0)
		mft_idx := int64(arg.MFT)

		if arg.Inode != "" {
			mft_idx, attr_type, attr_id, err = ntfs.ParseMFTId(arg.Inode)
			if err != nil {
				scope.Log("parse_ntfs_ranges: %v", err)
				return
			}
		} else {
			attr_type = 128
		}

		device, _, err := paths.GetDeviceAndSubpath(arg.Device)
		if err != nil {
			scope.Log("parse_ntfs_ranges: %v", err)
			return
		}

		ntfs_ctx, err := readers.GetNTFSContext(scope, device, "file")
		if err != nil {
			scope.Log("parse_ntfs_ranges: %v", err)
			return
		}

		if arg.MFTOffset > 0 {
			mft_idx = arg.MFTOffset / ntfs_ctx.Boot.ClusterSize()
		}

		mft_entry, err := ntfs_ctx.GetMFT(mft_idx)
		if err != nil {
			scope.Log("parse_ntfs_ranges: %v", err)
			return
		}

		reader, err := ntfs.OpenStream(ntfs_ctx, mft_entry,
			uint64(attr_type), uint16(attr_id))
		if err != nil {
			scope.Log("parse_ntfs_ranges: %v", err)
			return
		}

		for _, rng := range reader.Ranges() {
			select {
			case <-ctx.Done():
				return

			case output_chan <- rng:
			}
		}
	}()

	return output_chan
}

func (self NTFSRangesPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "parse_ntfs_ranges",
		Doc:     "Show the run ranges for an NTFS stream.",
		ArgType: type_map.AddType(scope, &NTFSFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&NTFSFunction{})
	vql_subsystem.RegisterPlugin(&NTFSI30ScanPlugin{})
	vql_subsystem.RegisterPlugin(&MFTScanPlugin{})
	vql_subsystem.RegisterPlugin(&NTFSRangesPlugin{})
}
