//go:build windows
// +build windows

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
// Windows specific implementation. For windows we make a special
// virtual root directory which contains all the drives as if they are
// subdirs. For example list dir "\\" yields c:, d:, e: then we access
// each file as an absolute path: \\c:\\Windows -> c:\Windows.
package file

import (
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/Velocidex/ordereddict"
	errors "github.com/go-errors/errors"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

var (
	fileAccessorCurrentOpened = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "accessor_file_current_open",
		Help: "Number of currently opened files with the file accessor.",
	})
)

type OSFileInfo struct {
	os.FileInfo

	// Empty for files but may contain data for registry and
	// resident NTFS.
	_full_path *accessors.OSPath

	follow_links bool
}

func NewOSFileInfo(base os.FileInfo, path *accessors.OSPath) *OSFileInfo {
	return &OSFileInfo{
		FileInfo:   base,
		_full_path: path,
	}
}

func (self *OSFileInfo) FullPath() string {
	return self._full_path.String()
}

func (self *OSFileInfo) OSPath() *accessors.OSPath {
	return self._full_path
}

func (self *OSFileInfo) Data() *ordereddict.Dict {
	if self.IsLink() {
		target, err := os.Readlink(self.FullPath())
		if err == nil {
			return ordereddict.NewDict().
				Set("Link", target)
		}
	}
	return ordereddict.NewDict()
}

func (self *OSFileInfo) Btime() time.Time {
	nsec := self.sys().CreationTime.Nanoseconds()
	return time.Unix(0, nsec)
}

func (self *OSFileInfo) Mtime() time.Time {
	nsec := self.sys().LastWriteTime.Nanoseconds()
	return time.Unix(0, nsec)
}

// Windows does not provide the ctime (inode change time) using the
// APIs.
func (self *OSFileInfo) Ctime() time.Time {
	nsec := self.sys().LastWriteTime.Nanoseconds()
	return time.Unix(0, nsec)
}

func (self *OSFileInfo) Atime() time.Time {
	nsec := self.sys().LastAccessTime.Nanoseconds()
	return time.Unix(0, nsec)
}

func (self *OSFileInfo) IsLink() bool {
	return self.Mode()&os.ModeSymlink != 0
}

func (self *OSFileInfo) GetLink() (*accessors.OSPath, error) {
	if !self.follow_links {
		return nil, errors.New("Not following links")
	}

	target, err := os.Readlink(self.FullPath())
	if err != nil {
		return nil, err
	}
	return self._full_path.Parse(target)
}

func (self *OSFileInfo) sys() *syscall.Win32FileAttributeData {
	return self.Sys().(*syscall.Win32FileAttributeData)
}

type OSFileSystemAccessor struct {
	follow_links bool
	scope        vfilter.Scope
}

func (self OSFileSystemAccessor) ParsePath(path string) (
	*accessors.OSPath, error) {
	return accessors.NewWindowsOSPath(path)
}

func (self OSFileSystemAccessor) New(scope vfilter.Scope) (
	accessors.FileSystemAccessor, error) {

	return &OSFileSystemAccessor{
		follow_links: self.follow_links,
		scope:        scope,
	}, nil
}

func (self OSFileSystemAccessor) Describe() *accessors.AccessorDescriptor {
	return &accessors.AccessorDescriptor{
		Name:        "file",
		Description: `Access the filesystem using the OS API.`,
		Permissions: []acls.ACL_PERMISSION{acls.FILESYSTEM_READ},
	}
}

func (self *OSFileSystemAccessor) GetUnderlyingAPIFilename(
	full_path *accessors.OSPath) (string, error) {
	return full_path.PathSpec().Path, nil
}

func (self OSFileSystemAccessor) ReadDir(path string) (
	[]accessors.FileInfo, error) {
	full_path, err := self.ParsePath(path)
	if err != nil {
		return nil, err
	}

	return self.ReadDirWithOSPath(full_path)
}

// On Windows filesystems are usually case insensitive.
func (self OSFileSystemAccessor) GetCanonicalFilename(
	path *accessors.OSPath) string {
	return strings.ToLower(path.String())
}

func (self OSFileSystemAccessor) ReadDirWithOSPath(
	full_path *accessors.OSPath) ([]accessors.FileInfo, error) {
	var result []accessors.FileInfo

	defer Instrument("ReadDirWithOSPath")()

	err := CheckPrefix(full_path)
	if err != nil {
		return nil, err
	}

	// No drive part, so list all drives.
	if len(full_path.Components) == 0 {
		return Cache.DiscoverDriveLetters()
	}

	// Add a final \ to turn path into a directory path. This is
	// needed for windows since paths that do not end with a \\
	// are interpreted incorrectly. Example readdir("c:") is not
	// the same as readdir("c:\\")
	dir_path := full_path.String() + "\\"

	// Windows symlinks are buggy - a ReadDir() of a link to a
	// directory fails and the caller needs to specially check for
	// a link. Only file access through the symlink works as
	// expected (e.g. if link is a symlink to C:\Program Files):

	// dir link
	// 01/15/2019  03:32 AM    <SYMLINK>      link [c:\Program Files]

	// dir link\
	// File Not Found     <-- this should work to list the content of the
	//                        link target

	// dir link\Git
	// Content of Git directory.

	// For this reason we need to take special care when reading a
	// directory in case that directory itself is a link.
	files, err := utils.ReadDir(dir_path)
	if err != nil {
		if !self.follow_links {
			return nil, err
		}

		// Maybe it is a symlink
		link_path := full_path.String()
		target, err := os.Readlink(link_path)
		if err == nil {

			// Yes it is a symlink, we just recurse into
			// the target
			files, err = utils.ReadDir(target)
		}
	}

	for _, f := range files {
		child_path := full_path.Append(f.Name())
		err := CheckPrefix(child_path)
		if err != nil {
			continue
		}

		result = append(result,
			&OSFileInfo{
				follow_links: self.follow_links,
				FileInfo:     f,
				_full_path:   child_path,
			})
	}
	return result, nil
}

// Wrap the os.File object to keep track of open file handles.
type OSFileWrapper struct {
	*os.File
}

func (self OSFileWrapper) IsSeekable() bool {
	return true
}

func (self OSFileWrapper) Close() error {
	fileAccessorCurrentOpened.Dec()
	return self.File.Close()
}

func (self OSFileSystemAccessor) Open(path string) (accessors.ReadSeekCloser, error) {
	full_path, err := self.ParsePath(path)
	if err != nil {
		return nil, err
	}
	return self.OpenWithOSPath(full_path)
}

func (self OSFileSystemAccessor) OpenWithOSPath(full_path *accessors.OSPath) (
	accessors.ReadSeekCloser, error) {

	defer Instrument("OpenWithOSPath")()

	err := CheckPrefix(full_path)
	if err != nil {
		return nil, err
	}

	// Opening the drive letter directly produces a reader over the
	// raw disk.
	if len(full_path.Components) == 1 {
		device_name := full_path.Components[0]
		return getDeviceReader(self.scope, device_name)
	}

	filename := full_path.String()

	// The API does not accept filenames with trailing \\ for an open call.
	filename = strings.TrimSuffix(filename, "\\")
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	fileAccessorCurrentOpened.Inc()
	return OSFileWrapper{file}, err
}

func (self *OSFileSystemAccessor) Lstat(path string) (accessors.FileInfo, error) {
	defer Instrument("Lstat")()

	full_path, err := self.ParsePath(path)
	if err != nil {
		return nil, err
	}

	err = CheckPrefix(full_path)
	if err != nil {
		return nil, err
	}

	stat, err := os.Lstat(full_path.String())
	return &OSFileInfo{
		follow_links: self.follow_links,
		FileInfo:     stat,
		_full_path:   full_path,
	}, err
}

func (self *OSFileSystemAccessor) LstatWithOSPath(full_path *accessors.OSPath) (
	accessors.FileInfo, error) {

	defer Instrument("LstatWithOSPath")()

	err := CheckPrefix(full_path)
	if err != nil {
		return nil, err
	}

	// An Lstat of a device returns metadata about the device
	if len(full_path.Components) == 1 {
		devices, err := Cache.DiscoverDriveLetters()
		if err != nil {
			return nil, err
		}

		// Find the right device information
		for _, d := range devices {
			if full_path.Components[0] == d.Name() {
				return d, nil
			}
		}
		return nil, utils.NotFoundError
	}

	stat, err := os.Lstat(full_path.String())
	return &OSFileInfo{
		follow_links: self.follow_links,
		FileInfo:     stat,
		_full_path:   full_path,
	}, err
}

func init() {
	accessors.Register(&OSFileSystemAccessor{})

	// Windows filesystem is already case insensitive so we provide an
	// alias so artifacts can work with either.
	accessors.Register(accessors.DescribeAccessor(
		&OSFileSystemAccessor{}, accessors.AccessorDescriptor{
			Name:        "file_nocase",
			Description: `Access the filesystem using the OS API.`,
		}))

	// Register a variant which allows following links - be
	// careful with it - it can get stuck on loops.
	accessors.Register(accessors.DescribeAccessor(
		&OSFileSystemAccessor{
			follow_links: true,
		}, accessors.AccessorDescriptor{
			Name: "file_links",
			Description: `Access the filesystem using the OS API.
This Accessor also follows any symlinks - Note: Take care with this accessor because there may be circular links.
`,
			Permissions: []acls.ACL_PERMISSION{acls.FILESYSTEM_READ},
		}))

	// We do not register the OSFileSystemAccessor directly - it
	// is used through the AutoFilesystemAccessor: If we can not
	// open the file with regular OS APIs we fallback to raw NTFS
	// access. This is usually what we want.
	json.RegisterCustomEncoder(&OSFileInfo{}, accessors.MarshalGlobFileInfo)
}
