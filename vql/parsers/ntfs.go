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
package parsers

import (
	"context"
	"errors"
	"strings"

	"github.com/Velocidex/ordereddict"
	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/ntfs/readers"
	utils "www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type NTFSFunctionArgs struct {
	Device    string            `vfilter:"optional,field=device,doc=The device file to open. This may be a full path for example C:\\Windows - we will figure out the device automatically."`
	Filename  *accessors.OSPath `vfilter:"optional,field=filename,doc=A raw image to open. You can also provide the accessor if using a raw image file."`
	Accessor  string            `vfilter:"optional,field=accessor,doc=The accessor to use."`
	Inode     string            `vfilter:"optional,field=inode,doc=The MFT entry to parse in inode notation (5-144-1)."`
	MFT       int64             `vfilter:"optional,field=mft,doc=The MFT entry to parse."`
	MFTOffset int64             `vfilter:"optional,field=mft_offset,doc=The offset to the MFT entry to parse."`
}

type NTFSModel struct {
	*ntfs.NTFSFileInformation

	Device *accessors.OSPath
}

type NTFSFunction struct{}

func (self NTFSFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "parse_ntfs",
		Doc:     "Parse specific inodes from an NTFS image file or the raw device.",
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

	arg.Filename, arg.Accessor, err = getOSPathAndAccessor(arg.Device,
		arg.Filename, arg.Accessor)
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

	ntfs_ctx, err := readers.GetNTFSContext(scope, arg.Filename, arg.Accessor)
	if err != nil {
		scope.Log("parse_ntfs: GetNTFSContext %v", err)
		return &vfilter.Null{}
	}
	defer ntfs_ctx.Close()

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

	return &NTFSModel{NTFSFileInformation: result, Device: arg.Filename}
}

type MFTScanPluginArgs struct {
	Filename string `vfilter:"required,field=filename,doc=The MFT file."`
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

		accessor, err := accessors.GetAccessor(arg.Accessor, scope)
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

		st, err := accessor.Lstat(arg.Filename)
		if err != nil {
			scope.Log("parse_mft: Unable to open file %s: %v",
				arg.Filename, err)
			return
		}

		for item := range ntfs.ParseMFTFile(
			ctx, utils.ReaderAtter{Reader: fd}, st.Size(), 0x1000, 0x400) {
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

		arg.Filename, arg.Accessor, err = getOSPathAndAccessor(arg.Device,
			arg.Filename, arg.Accessor)
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

		ntfs_ctx, err := readers.GetNTFSContext(scope, arg.Filename, arg.Accessor)
		if err != nil {
			scope.Log("parse_ntfs_i30: %v", err)
			return
		}
		defer ntfs_ctx.Close()

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

		arg.Filename, arg.Accessor, err = getOSPathAndAccessor(arg.Device,
			arg.Filename, arg.Accessor)
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

		ntfs_ctx, err := readers.GetNTFSContext(scope, arg.Filename, arg.Accessor)
		if err != nil {
			scope.Log("parse_ntfs_ranges: %v", err)
			return
		}
		defer ntfs_ctx.Close()

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

func getOSPathAndAccessor(
	device string,
	filename *accessors.OSPath,
	accessor string) (*accessors.OSPath, string, error) {

	// Extract the device from the device string
	if device != "" {
		filename, err := accessors.NewWindowsNTFSPath(device)
		if err != nil {
			return nil, "", err
		}

		if filename == nil || len(filename.Components) == 0 {
			return nil, "", errors.New("Invalid device")
		}

		if !strings.HasPrefix(filename.Components[0], "\\\\.\\") {
			return nil, "", errors.New("Device should begin with \\\\.\\")
		}

		// The device is the first component (e.g. \\.\C:) so make a
		// new OSPath for it to be accessed using "ntfs".
		filename, err = accessors.NewWindowsNTFSPath(filename.Components[0])
		accessor = "ntfs"
		return filename, accessor, nil
	}

	if filename == nil {
		return nil, "", errors.New("either filename or device must be provided")
	}

	return filename, accessor, nil
}

func init() {
	vql_subsystem.RegisterFunction(&NTFSFunction{})
	vql_subsystem.RegisterPlugin(&NTFSI30ScanPlugin{})
	vql_subsystem.RegisterPlugin(&MFTScanPlugin{})
	vql_subsystem.RegisterPlugin(&NTFSRangesPlugin{})
}
