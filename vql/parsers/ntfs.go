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
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Velocidex/ordereddict"
	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	"www.velocidex.com/golang/velociraptor/glob"
	utils "www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

var (
	// For convenience we transform paths like c:\Windows -> \\.\c:\Windows
	driveRegex = regexp.MustCompile(
		`(?i)^[/\\]?([a-z]:)(.*)`)
	deviceDriveRegex = regexp.MustCompile(
		`(?i)^(\\\\[\?\.]\\[a-zA-Z]:)(.*)`)
	deviceDirectoryRegex = regexp.MustCompile(
		`(?i)^(\\\\[\?\.]\\GLOBALROOT\\Device\\[^/\\]+)([/\\]?.*)`)
)

func GetDeviceAndSubpath(path string) (device string, subpath string, err error) {
	// Make sure not to run filepath.Clean() because it will
	// collapse multiple slashes (and prevent device names from
	// being recognized).
	path = strings.Replace(path, "/", "\\", -1)

	m := deviceDriveRegex.FindStringSubmatch(path)
	if len(m) != 0 {
		return m[1], clean(m[2]), nil
	}

	m = driveRegex.FindStringSubmatch(path)
	if len(m) != 0 {
		return "\\\\.\\" + m[1], clean(m[2]), nil
	}

	m = deviceDirectoryRegex.FindStringSubmatch(path)
	if len(m) != 0 {
		return m[1], clean(m[2]), nil
	}

	return "/", path, errors.New("Unsupported device type")
}

func clean(path string) string {
	result := filepath.Clean(path)
	if result == "." {
		result = ""
	}

	return result
}

func GetNTFSContext(scope *vfilter.Scope, device string) (*ntfs.NTFSContext, error) {
	ntfs_ctx, ok := vql_subsystem.CacheGet(scope, device).(*ntfs.NTFSContext)
	if !ok {
		fd, err := os.OpenFile(device, os.O_RDONLY, os.FileMode(0666))
		if err != nil {
			return nil, err
		}

		scope.AddDestructor(func() { fd.Close() })
		paged_reader, err := ntfs.NewPagedReader(fd, 1024, 10000)
		if err != nil {
			return nil, err
		}

		ntfs_ctx, err = ntfs.GetNTFSContext(paged_reader, 0)
		if err != nil {
			return nil, err
		}

		vql_subsystem.CacheSet(scope, device, ntfs_ctx)
	}

	return ntfs_ctx, nil
}

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

func (self NTFSFunction) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "parse_ntfs",
		Doc:     "Parse an NTFS image file.",
		ArgType: type_map.AddType(scope, &NTFSFunctionArgs{}),
	}
}

func (self NTFSFunction) Call(
	ctx context.Context, scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &NTFSFunctionArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
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

	device, _, err := GetDeviceAndSubpath(arg.Device)
	if err != nil {
		scope.Log("parse_ntfs: %v", err)
		return &vfilter.Null{}
	}

	ntfs_ctx, err := GetNTFSContext(scope, device)
	if err != nil {
		scope.Log("parse_ntfs: %v", err)
		return &vfilter.Null{}
	}

	if arg.MFTOffset > 0 {
		arg.MFT = arg.MFTOffset / ntfs_ctx.Boot.ClusterSize()
	}

	mft_entry, err := ntfs_ctx.GetMFT(arg.MFT)
	if err != nil {
		scope.Log("parse_ntfs: %v", err)
		return &vfilter.Null{}
	}

	result, err := ntfs.ModelMFTEntry(ntfs_ctx, mft_entry)
	if err != nil {
		scope.Log("parse_ntfs: %v", err)
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
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &MFTScanPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("parse_mft: %v", err)
			return
		}

		accessor, err := glob.GetAccessor(arg.Accessor, ctx)
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
			utils.ReaderAtter{fd}, 1024, 10000)
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
			reader, st.Size(), 0x1000, 0x400) {
			output_chan <- item
		}
	}()

	return output_chan
}

func (self MFTScanPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "parse_mft",
		Doc:     "Scan the $MFT from an NTFS volume.",
		ArgType: type_map.AddType(scope, &MFTScanPluginArgs{}),
	}
}

type NTFSI30ScanPlugin struct{}

func (self NTFSI30ScanPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &NTFSFunctionArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
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

		ntfs_ctx, err := GetNTFSContext(scope, arg.Device)
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
			output_chan <- fileinfo
		}
	}()

	return output_chan
}

func (self NTFSI30ScanPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "parse_ntfs_i30",
		Doc:     "Scan the $I30 stream from an NTFS MFT entry.",
		ArgType: type_map.AddType(scope, &NTFSFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&NTFSFunction{})
	vql_subsystem.RegisterPlugin(&NTFSI30ScanPlugin{})
	vql_subsystem.RegisterPlugin(&MFTScanPlugin{})
}
