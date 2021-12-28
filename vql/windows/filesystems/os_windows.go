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
// Windows specific implementation. For windows we make a special
// virtual root directory which contains all the drives as if they are
// subdirs. For example list dir "\\" yields c:, d:, e: then we access
// each file as an absolute path: \\c:\\Windows -> c:\Windows.
package filesystems

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Velocidex/ordereddict"
	errors "github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/shirou/gopsutil/v3/disk"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/windows/wmi"
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
	_full_path string

	follow_links bool
}

func (self *OSFileInfo) FullPath() string {
	return self._full_path
}

func (self *OSFileInfo) Data() interface{} {
	if self.IsLink() {
		path := strings.TrimRight(
			strings.TrimLeft(self.FullPath(), "\\"), "\\")
		target, err := os.Readlink(path)
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

func (self *OSFileInfo) GetLink() (string, error) {
	if !self.follow_links {
		return "", errors.New("Not following links")
	}

	path := strings.TrimRight(
		strings.TrimLeft(self.FullPath(), "\\"), "\\")
	target, err := os.Readlink(path)
	if err != nil {
		return "", err
	}
	return target, nil
}

func (self *OSFileInfo) sys() *syscall.Win32FileAttributeData {
	return self.Sys().(*syscall.Win32FileAttributeData)
}

func getAvailableDrives() ([]string, error) {
	partitions, err := disk.Partitions(true)
	if err != nil {
		return nil, err
	}
	result := []string{}
	for _, item := range partitions {
		// TODO: Filter only local filesystems vs. network.
		result = append(result, item.Device)
	}

	return result, nil
}

// Glob sends us paths in normal form which we need to convert to
// windows form. Normal form uses / instead of \ and always has a
// leading /.
func GetPath(path string) string {
	if strings.HasPrefix(path, "\\\\") {
		return path
	}

	path = strings.Replace(path, "/", "\\", -1)

	// Strip leading \\ so \\c:\\windows -> c:\\windows
	path = strings.TrimLeft(path, "\\")
	if path == "." {
		return ""
	}
	return path
}

type OSFileSystemAccessor struct {
	follow_links bool
}

func (self OSFileSystemAccessor) New(scope vfilter.Scope) (glob.FileSystemAccessor, error) {
	result := &OSFileSystemAccessor{follow_links: self.follow_links}
	return result, nil
}

func discoverDriveLetters() ([]glob.FileInfo, error) {
	result := []glob.FileInfo{}

	shadow_volumes, err := wmi.Query(
		"SELECT DeviceID, Description, VolumeName, FreeSpace, "+
			"Size, SystemName, VolumeSerialNumber "+
			"from Win32_LogicalDisk",
		"ROOT\\CIMV2")
	if err == nil {
		for _, row := range shadow_volumes {
			size, _ := row.GetInt64("Size")
			device_name, pres := row.GetString("DeviceID")
			if pres {
				virtual_directory := glob.NewVirtualDirectoryPath(
					escape(device_name), row, size, os.ModeDir)
				result = append(result, virtual_directory)
			}
		}
	}

	return result, nil
}

func (self OSFileSystemAccessor) ReadDir(path string) ([]glob.FileInfo, error) {
	return self.readDir(path, 0)
}

func (self OSFileSystemAccessor) readDir(path string, depth int) ([]glob.FileInfo, error) {
	var result []glob.FileInfo

	if depth > 10 {
		return nil, errors.New("Too many symlinks.")
	}

	// No drive part, so list all drives.
	if path == "/" {
		return discoverDriveLetters()
	}

	// Add a final \ to turn path into a directory path. This is
	// needed for windows since paths that do not end with a \\
	// are interpreted incorrectly. Example readdir("c:") is not
	// the same as readdir("c:\\")
	dir_path := GetPath(path) + "\\"

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
		link_path := GetPath(path)
		target, err := os.Readlink(link_path)
		if err == nil {

			// Yes it is a symlink, we just recurse into
			// the target
			files, err = utils.ReadDir(target)
		}
	}

	for _, f := range files {
		result = append(result,
			&OSFileInfo{
				follow_links: self.follow_links,
				FileInfo:     f,
				_full_path:   dir_path + f.Name(),
			})
	}
	return result, nil
}

// Wrap the os.File object to keep track of open file handles.
type OSFileWrapper struct {
	*os.File
}

func (self OSFileWrapper) Close() error {
	fileAccessorCurrentOpened.Dec()
	return self.File.Close()
}

func (self OSFileSystemAccessor) Open(path string) (glob.ReadSeekCloser, error) {
	path = GetPath(path)
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	fileAccessorCurrentOpened.Inc()
	return OSFileWrapper{file}, err
}

func (self *OSFileSystemAccessor) Lstat(path string) (glob.FileInfo, error) {

	stat, err := os.Lstat(GetPath(path))
	return &OSFileInfo{
		follow_links: self.follow_links,
		FileInfo:     stat,
		_full_path:   path,
	}, err
}

func (self *OSFileSystemAccessor) GetRoot(path string) (string, string, error) {
	return "/", path, nil
}

// We accept both / and \ as a path separator
func (self *OSFileSystemAccessor) PathSplit(path string) []string {
	return NTFSFileSystemAccessor_re.Split(path, -1)
}

func (self *OSFileSystemAccessor) PathJoin(x, y string) string {
	return filepath.Join(x, y)
}

func init() {
	// Register a variant which allows following links - be
	// careful with it - it can get stuck on loops.
	glob.Register("file_links", &OSFileSystemAccessor{follow_links: true}, `Access the filesystem using the OS APIs.

This Accessor also follows any symlinks - Note: Take care with this accessor because there may be circular links.`)

	// We do not register the OSFileSystemAccessor directly - it
	// is used through the AutoFilesystemAccessor: If we can not
	// open the file with regular OS APIs we fallback to raw NTFS
	// access. This is usually what we want.

	json.RegisterCustomEncoder(&OSFileInfo{}, glob.MarshalGlobFileInfo)
}
