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
// filesystem by MFT ids e.g. C:/X-Y-Z

// The First level is the MFT ID (X)
// The Second level is the Attribute type (Y)
// The Third level is the Attribute ID.

package ntfs

import (
	"errors"
	"os"

	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/ntfs/readers"
	"www.velocidex.com/golang/vfilter"
)

type MFTFileSystemAccessor struct {
	scope vfilter.Scope
}

func (self MFTFileSystemAccessor) ParsePath(path string) (
	*accessors.OSPath, error) {
	return accessors.NewWindowsNTFSPath(path)
}

func (self MFTFileSystemAccessor) New(scope vfilter.Scope) (
	accessors.FileSystemAccessor, error) {
	return &MFTFileSystemAccessor{scope: scope}, nil
}

func (self MFTFileSystemAccessor) ReadDir(path string) (
	[]accessors.FileInfo, error) {
	return nil, errors.New("Unable to list all MFT entries.")
}

func (self MFTFileSystemAccessor) ReadDirWithOSPath(path *accessors.OSPath) (
	[]accessors.FileInfo, error) {
	return nil, errors.New("Unable to list all MFT entries.")
}

func (self MFTFileSystemAccessor) parseMFTPath(full_path *accessors.OSPath) (
	delegate_device *accessors.OSPath, delegate_accessor string,
	subpath string, err error) {

	// There are two ways to use this accessor:

	// 1. Using a pathspec we can delegate to an external file to
	//    parse the ntfs. Eg. {Path: "43-128-0", DelegatePath: "\\\\.\\C:"}
	// 2. If a delegate is not specified, we take the device from the
	//    first component of the Path.

	delegate_device = full_path.Clear().Append(full_path.Components[0])
	delegate_accessor = "file"

	// If the user provided a full pathspec we use that instead.
	if full_path.DelegatePath() != "" {
		delegate_device, err = full_path.Delegate(self.scope)
		if err != nil {
			return nil, "", "", err
		}
		delegate_accessor = full_path.DelegateAccessor()
		subpath = full_path.Components[0]
	} else if len(full_path.Components) < 2 {
		return nil, "", "", os.ErrNotExist
	} else {
		subpath = full_path.Components[1]
	}
	return delegate_device, delegate_accessor, subpath, nil
}

func (self *MFTFileSystemAccessor) Open(path string) (
	accessors.ReadSeekCloser, error) {

	full_path, err := self.ParsePath(path)
	if err != nil || len(full_path.Components) == 0 {
		return nil, os.ErrNotExist
	}

	return self.OpenWithOSPath(full_path)
}

func (self *MFTFileSystemAccessor) OpenWithOSPath(full_path *accessors.OSPath) (
	accessors.ReadSeekCloser, error) {

	delegate_device, delegate_accessor, subpath, err := self.parseMFTPath(
		full_path)
	if err != nil {
		return nil, err
	}

	// Check that the subpath is correctly specified.
	mft_idx, attr_type, attr_id, err := ntfs.ParseMFTId(subpath)
	if err != nil {
		return nil, err
	}

	ntfs_ctx, err := readers.GetNTFSContext(
		self.scope, delegate_device, delegate_accessor)
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
			_full_path: full_path.Copy(),
		},
		reader: reader,
	}
	return result, nil
}

func (self *MFTFileSystemAccessor) Lstat(path string) (
	accessors.FileInfo, error) {
	full_path, err := self.ParsePath(path)
	if err != nil || len(full_path.Components) == 0 {
		return nil, os.ErrNotExist
	}

	return self.LstatWithOSPath(full_path)
}

func (self *MFTFileSystemAccessor) LstatWithOSPath(full_path *accessors.OSPath) (
	accessors.FileInfo, error) {
	delegate_device, delegate_accessor, subpath, err := self.parseMFTPath(full_path)
	if err != nil {
		return nil, err
	}

	// Check that the subpath is correctly specified.
	mft_idx, _, _, err := ntfs.ParseMFTId(subpath)
	if err != nil {
		return nil, err
	}

	ntfs_ctx, err := readers.GetNTFSContext(
		self.scope, delegate_device, delegate_accessor)
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

	return &NTFSFileInfo{
		info:       info,
		_full_path: full_path.Copy(),
	}, nil
}

func init() {
	accessors.Register("mft", &MFTFileSystemAccessor{},
		`Access arbitrary MFT streams as files.

The filename is taken as an MFT inode number in the
form <entry_id>-<stream_type>-<id>, e.g. 203-128-0

An example of using this artifact:

SELECT upload(accessor="mft", filename="C:/203-128-0")
FROM scope()

`)
}
