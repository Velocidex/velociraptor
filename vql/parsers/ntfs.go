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
package parsers

import (
	"context"
	"errors"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/go-ntfs/parser"
	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/ntfs/readers"
	"www.velocidex.com/golang/velociraptor/acls"
	utils "www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vql_readers "www.velocidex.com/golang/velociraptor/vql/readers"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type NTFSFunctionArgs struct {
	Device      string            `vfilter:"optional,field=device,doc=The device file to open. This may be a full path for example C:\\Windows - we will figure out the device automatically."`
	Filename    *accessors.OSPath `vfilter:"optional,field=filename,doc=A raw image to open. You can also provide the accessor if using a raw image file."`
	MFTFilename *accessors.OSPath `vfilter:"optional,field=mft_filename,doc=A path to a raw $MFT file to parse."`
	Accessor    string            `vfilter:"optional,field=accessor,doc=The accessor to use."`
	Inode       string            `vfilter:"optional,field=inode,doc=The MFT entry to parse in inode notation (5-144-1)."`
	MFT         int64             `vfilter:"optional,field=mft,doc=The MFT entry to parse."`
	MFTOffset   int64             `vfilter:"optional,field=mft_offset,doc=The offset to the MFT entry to parse."`
}

func (self *NTFSFunctionArgs) getNTFSContext(
	scope vfilter.Scope) (ntfs_ctx *parser.NTFSContext, err error) {

	// Normalize some other args
	if self.Inode != "" {
		mft_idx, _, _, _, err := ntfs.ParseMFTId(self.Inode)
		if err != nil {
			return nil, err
		}
		self.MFT = mft_idx

	}

	if self.Device != "" {
		ntfs_ctx, self.Filename, self.Accessor, err = getNTFSContextFromDevice(
			scope, self.Device)

	} else if self.Filename != nil {
		ntfs_ctx, err = getNTFSContextFromImage(scope, self.Filename, self.Accessor)

	} else if self.MFTFilename != nil {
		ntfs_ctx, err = getNTFSContextFromMFT(scope, self.MFTFilename, self.Accessor)

	} else {
		return nil, errors.New(
			"Either filename, mft_filename or device must be specified")
	}

	if err != nil {
		return nil, err
	}

	if self.MFTOffset > 0 {
		self.MFT = self.MFTOffset / ntfs_ctx.ClusterSize
	}

	return ntfs_ctx, err
}

type NTFSModel struct {
	*ntfs.NTFSFileInformation

	Device *accessors.OSPath
	OSPath *accessors.OSPath
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
	defer vql_subsystem.RegisterMonitor(ctx, "parse_ntfs", args)()

	arg := &NTFSFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("parse_ntfs: %v", err)
		return &vfilter.Null{}
	}

	ntfs_ctx, err := arg.getNTFSContext(scope)
	if err != nil {
		scope.Log("parse_ntfs: %v", err)
		return &vfilter.Null{}
	}
	defer ntfs_ctx.Close()

	if arg.Inode != "" {
		mft_idx, _, _, _, err := ntfs.ParseMFTId(arg.Inode)
		if err != nil {
			scope.Log("parse_ntfs: %v", err)
			return &vfilter.Null{}
		}
		arg.MFT = mft_idx
	}

	if ntfs_ctx == nil {
		scope.Log("parse_ntfs: invalid context")
		return &vfilter.Null{}
	}

	if arg.MFTOffset > 0 {
		arg.MFT = arg.MFTOffset / ntfs_ctx.ClusterSize
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

	// Come up with a reasonable value for OSPath in the
	// output. Typically we can use this to open the actual file, but
	// if the context is really a raw $MFT file then we can not
	// actually open any file.
	var ospath *accessors.OSPath

	// A Device was given the OSPath should be relative to the device
	// so it can be opened by the 'ntfs' accessor
	if arg.Device != "" && arg.Filename != nil {
		if len(result.Hardlinks) > 0 {
			ospath = arg.Filename.Append(strings.Split(result.Hardlinks[0], "\\")...)
		}

	} else if arg.Filename != nil {

		// A filename was given - we just return the OSPath relative
		// to the root of the filesystem. This can be used to open the
		// file with the 'raw_ntfs' accessor.
		if len(result.Hardlinks) > 0 {
			ospath, _ = accessors.NewWindowsNTFSPath("")
			err = ospath.SetPathSpec(&accessors.PathSpec{
				DelegateAccessor: arg.Accessor,
				DelegatePath:     arg.Filename.Path(),
				Path:             result.Hardlinks[0],
			})
			if err != nil {
				scope.Log("parse_ntfs: SetPathSpec %v", err)
				return &vfilter.Null{}
			}
		}

		// An MFT file was given, cant really open the file anyway.
	} else if arg.MFTFilename != nil {
		if len(result.Hardlinks) > 0 {
			ospath, _ = accessors.NewWindowsNTFSPath("")
			err = ospath.SetPathSpec(&accessors.PathSpec{
				Path: result.Hardlinks[0],
			})
			if err != nil {
				scope.Log("parse_ntfs: SetPathSpec %v", err)
				return &vfilter.Null{}
			}
		}
	}

	return &NTFSModel{
		NTFSFileInformation: result,
		Device:              arg.Filename,
		OSPath:              ospath,
	}
}

type MFTScanPluginArgs struct {
	Filename   *accessors.OSPath `vfilter:"required,field=filename,doc=The MFT file."`
	Accessor   string            `vfilter:"optional,field=accessor,doc=The accessor to use."`
	Prefix     *accessors.OSPath `vfilter:"optional,field=prefix,doc=If specified we prefix all paths with this path."`
	StartEntry int64             `vfilter:"optional,field=start,doc=The first entry to scan."`
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
		defer vql_subsystem.RegisterMonitor(ctx, "parse_mft", args)()

		arg := &MFTScanPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("parse_mft: %v", err)
			return
		}

		// Choose a managed reader to ensure it does not get closed prematurely.
		fd, err := vql_readers.NewAccessorReader(scope, arg.Accessor, arg.Filename, 1000)
		if err != nil {
			scope.Log("parse_mft: %v", err)
			return
		}
		defer fd.Close()

		accessor, err := accessors.GetAccessor(arg.Accessor, scope)
		if err != nil {
			scope.Log("parse_mft: %v", err)
			return
		}

		st, err := accessor.LstatWithOSPath(arg.Filename)
		if err != nil {
			scope.Log("parse_mft: Unable to open file %s: %v",
				arg.Filename, err)
			return
		}

		options := readers.GetScopeOptions(scope)
		if arg.Prefix != nil {
			options.PrefixComponents = arg.Prefix.Components
		}

		for item := range ntfs.ParseMFTFileWithOptions(
			ctx, fd, st.Size(),
			0x1000, 0x400, arg.StartEntry, options) {
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
		Name:     "parse_mft",
		Doc:      "Scan the $MFT from an NTFS volume.",
		ArgType:  type_map.AddType(scope, &MFTScanPluginArgs{}),
		Version:  2,
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
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
		defer vql_subsystem.RegisterMonitor(ctx, "parse_ntfs_i30", args)()

		arg := &NTFSFunctionArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("parse_ntfs_i30: %v", err)
			return
		}

		ntfs_ctx, err := arg.getNTFSContext(scope)
		if err != nil {
			scope.Log("parse_ntfs_i30: %v", err)
			return
		}
		defer ntfs_ctx.Close()

		mft_entry, err := ntfs_ctx.GetMFT(arg.MFT)
		if err != nil {
			scope.Log("parse_ntfs_i30: %v", err)
			return
		}

		for _, fileinfo := range ntfs.ExtractI30List(ntfs_ctx, mft_entry) {
			select {
			case <-ctx.Done():
				return

				// Full object is expanded through the
				// _MFTHighlightAssociative protocol
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
		defer vql_subsystem.RegisterMonitor(ctx, "parse_ntfs_ranges", args)()

		arg := &NTFSFunctionArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("parse_ntfs_ranges: %v", err)
			return
		}

		ntfs_ctx, err := arg.getNTFSContext(scope)
		if err != nil {
			scope.Log("parse_ntfs_ranges: %v", err)
			return
		}

		attr_type := int64(0)
		attr_id := int64(0)
		mft_idx := int64(arg.MFT)
		stream_name := ""

		// Callers can view ranges for any stream type.
		if arg.Inode != "" {
			mft_idx, attr_type, attr_id, stream_name, err = ntfs.ParseMFTId(arg.Inode)
			if err != nil {
				scope.Log("parse_ntfs_ranges: %v", err)
				return
			}

			// By default only view $DATA stream.
		} else {
			attr_type = 128
		}

		mft_entry, err := ntfs_ctx.GetMFT(mft_idx)
		if err != nil {
			scope.Log("parse_ntfs_ranges: %v", err)
			return
		}

		reader, err := ntfs.OpenStream(ntfs_ctx, mft_entry,
			uint64(attr_type), uint16(attr_id), stream_name)
		if err != nil {
			scope.Log("parse_ntfs_ranges: %v", err)
			return
		}

		for _, rng := range parser.DebugRuns(reader, 0) {
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

func getOSPathAndAccessor(device string) (*accessors.OSPath, string, error) {

	// Extract the device from the device string
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
	return filename, "ntfs", err
}

func init() {
	vql_subsystem.RegisterFunction(&NTFSFunction{})
	vql_subsystem.RegisterPlugin(&NTFSI30ScanPlugin{})
	vql_subsystem.RegisterPlugin(&MFTScanPlugin{})
	vql_subsystem.RegisterPlugin(&NTFSRangesPlugin{})
}
