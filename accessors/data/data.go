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
// A data filesystem accessor - allows data to be read as a file.

package data

import (
	"strings"

	"github.com/go-errors/errors"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/vfilter"
)

type DataFilesystemAccessor struct{}

func (self DataFilesystemAccessor) Describe() *accessors.AccessorDescriptor {
	return &accessors.AccessorDescriptor{
		Name:        "data",
		Description: `Makes a string appears as an in memory file. Path is taken as a literal string to use as the file's data`,
	}
}

func (self DataFilesystemAccessor) New(
	scope vfilter.Scope) (accessors.FileSystemAccessor, error) {
	return DataFilesystemAccessor{}, nil
}

// The path represent actual literal data so we parse it as a single
// component (It can not contain delegates for this accessor).
func (self DataFilesystemAccessor) ParsePath(
	path string) (*accessors.OSPath, error) {
	return accessors.MustNewPathspecOSPath("").Clear().Append(path), nil
}

func (self DataFilesystemAccessor) Lstat(
	filename string) (accessors.FileInfo, error) {
	full_path, err := self.ParsePath(filename)
	if err != nil {
		return nil, err
	}
	return &accessors.VirtualFileInfo{
		RawData: []byte(filename),
		Path:    full_path,
	}, nil
}

func (self DataFilesystemAccessor) LstatWithOSPath(
	full_path *accessors.OSPath) (accessors.FileInfo, error) {
	return &accessors.VirtualFileInfo{
		RawData: []byte(full_path.String()),
		Path:    full_path,
	}, nil
}

func (self DataFilesystemAccessor) ReadDir(
	path string) ([]accessors.FileInfo, error) {
	return nil, errors.New("Not implemented")
}

func (self DataFilesystemAccessor) ReadDirWithOSPath(
	path *accessors.OSPath) ([]accessors.FileInfo, error) {
	return nil, errors.New("Not implemented")
}

func (self DataFilesystemAccessor) Open(
	path string) (accessors.ReadSeekCloser, error) {
	return accessors.VirtualReadSeekCloser{
		ReadSeeker: strings.NewReader(path),
	}, nil
}

func (self DataFilesystemAccessor) OpenWithOSPath(
	path *accessors.OSPath) (accessors.ReadSeekCloser, error) {
	return accessors.VirtualReadSeekCloser{
		ReadSeeker: strings.NewReader(path.String()),
	}, nil
}

func init() {
	accessors.Register(&DataFilesystemAccessor{})
}
