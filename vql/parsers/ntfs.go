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
	"os"

	"www.velocidex.com/golang/go-ntfs"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

type NTFSFunctionArgs struct {
	Device    string `vfilter:"required,field=device,doc=The device file to open."`
	MFT       int64  `vfilter:"optional,field=mft,doc=The MFT entry to parse."`
	MFTOffset int64  `vfilter:"optional,field=mft_offset,doc=The offset to the MFT entry to parse."`
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
	args *vfilter.Dict) vfilter.Any {

	arg := &NTFSFunctionArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("parse_ntfs: %v", err)
		return &vfilter.Null{}
	}

	var boot *ntfs.NTFS_BOOT_SECTOR
	var ok bool

	boot, ok = vql_subsystem.CacheGet(scope, arg.Device).(*ntfs.NTFS_BOOT_SECTOR)
	if !ok {
		fd, err := os.OpenFile(arg.Device, os.O_RDONLY, os.FileMode(0666))
		if err != nil {
			scope.Log("parse_ntfs: %v", err)
			return &vfilter.Null{}
		}

		scope.AddDestructor(func() { fd.Close() })
		paged_reader, _ := ntfs.NewPagedReader(fd, 1024, 10000)

		profile, _ := ntfs.GetProfile()
		boot, err = ntfs.NewBootRecord(profile, paged_reader, 0)
		if err != nil {
			scope.Log("parse_ntfs: %v", err)
			return &vfilter.Null{}
		}

		vql_subsystem.CacheSet(scope, arg.Device, boot)
	}

	mft, err := boot.MFT()
	if err != nil {
		scope.Log("parse_ntfs: %v", err)
		return &vfilter.Null{}
	}

	if arg.MFTOffset > 0 {
		arg.MFT = arg.MFTOffset / boot.ClusterSize()
	}

	mft_entry, err := mft.MFTEntry(arg.MFT)
	if err != nil {
		scope.Log("parse_ntfs: %v", err)
		return &vfilter.Null{}
	}

	result, err := ntfs.ModelMFTEntry(mft_entry)
	if err != nil {
		scope.Log("parse_ntfs: %v", err)
		return &vfilter.Null{}
	}

	return result
}

func init() {
	vql_subsystem.RegisterFunction(&NTFSFunction{})
}
