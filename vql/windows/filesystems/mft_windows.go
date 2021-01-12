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
// A Raw NTFS accessor for disks. This accessor allows navigating the
// filesystem by MFT ids e.g. X/Y/Z

// The First level is the MFT ID (X)
// The Second level is the Attribute type (Y)
// The Third level is the Attribute ID.

package filesystems

import (
	"errors"
	"strings"

	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/vql/windows/filesystems/readers"
	"www.velocidex.com/golang/vfilter"
)

type MFTFileSystemAccessor struct {
	*NTFSFileSystemAccessor
}

func (self MFTFileSystemAccessor) New(scope vfilter.Scope) (glob.FileSystemAccessor, error) {
	ntfs_accessor, err := NTFSFileSystemAccessor{}.New(scope)
	if err != nil {
		return nil, err
	}
	return &MFTFileSystemAccessor{ntfs_accessor.(*NTFSFileSystemAccessor)}, nil
}

func (self MFTFileSystemAccessor) ReadDir(path string) ([]glob.FileInfo, error) {
	return nil, errors.New("Unable to list all MFT entries.")
}
func (self *MFTFileSystemAccessor) Open(path string) (glob.ReadSeekCloser, error) {
	// The path must start with a valid device, otherwise we list
	// the devices.
	device, subpath, err := self.GetRoot(path)
	if err != nil {
		return nil, errors.New("Unable to open raw device")
	}

	subpath = strings.TrimLeft(subpath, "\\")
	mft_idx, attr_type, attr_id, err := ntfs.ParseMFTId(subpath)
	if err != nil {
		return nil, err
	}

	ntfs_ctx, err := readers.GetNTFSContext(self.scope, device)
	if err != nil {
		return nil, err
	}

	mft_entry, err := ntfs_ctx.GetMFT(mft_idx)
	if err != nil {
		return nil, err
	}

	info := &ntfs.FileInfo{}
	stat := ntfs.Stat(ntfs_ctx, mft_entry)
	if len(stat) > 0 {
		info = stat[0]
	}

	// Attributes are never directories
	// since they always have some data.
	info.IsDir = false

	reader, err := ntfs.OpenStream(ntfs_ctx, mft_entry,
		uint64(attr_type), uint16(attr_id))
	if err != nil {
		return nil, err
	}

	ranges := reader.Ranges()
	if len(ranges) > 0 {
		last_run := ranges[len(ranges)-1]
		info.Size = last_run.Offset + last_run.Length
	}

	result := &readAdapter{
		info: &NTFSFileInfo{
			info:       info,
			_full_path: device + subpath,
		},
		reader: reader,
	}
	return result, nil
}

func (self *MFTFileSystemAccessor) Lstat(path string) (glob.FileInfo, error) {
	// The path must start with a valid device, otherwise we list
	// the devices.
	device, subpath, err := self.GetRoot(path)
	if err != nil {
		return nil, errors.New("Unable to open raw device")
	}

	subpath = strings.TrimLeft(subpath, "\\")
	mft_idx, _, _, err := ntfs.ParseMFTId(subpath)
	if err != nil {
		return nil, err
	}

	ntfs_ctx, err := readers.GetNTFSContext(self.scope, device)
	if err != nil {
		return nil, err
	}

	mft_entry, err := ntfs_ctx.GetMFT(mft_idx)
	if err != nil {
		return nil, err
	}

	var info *ntfs.FileInfo
	stat := ntfs.Stat(ntfs_ctx, mft_entry)
	if len(stat) > 0 {
		info = stat[0]
	}

	return &NTFSFileInfo{
		info:       info,
		_full_path: path,
	}, nil
}

func init() {
	glob.Register("mft", &MFTFileSystemAccessor{})
}
