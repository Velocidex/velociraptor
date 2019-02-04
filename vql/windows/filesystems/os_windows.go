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
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	errors "github.com/pkg/errors"
	"github.com/shirou/gopsutil/disk"
	"golang.org/x/sys/windows/registry"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/vql/windows/wmi"
)

type OSFileInfo struct {
	os.FileInfo

	// Empty for files but may contain data for registry and
	// resident NTFS.
	_data      string
	_full_path string
}

func (self *OSFileInfo) FullPath() string {
	return self._full_path
}

func (self *OSFileInfo) Data() interface{} {
	return self._data
}

func (self *OSFileInfo) Mtime() glob.TimeVal {
	nsec := self.sys().LastWriteTime.Nanoseconds()
	return glob.TimeVal{
		Sec:  nsec / 1000000000,
		Nsec: nsec,
	}
}

func (self *OSFileInfo) Ctime() glob.TimeVal {
	nsec := self.sys().CreationTime.Nanoseconds()
	return glob.TimeVal{
		Sec:  nsec / 1000000000,
		Nsec: nsec,
	}
}

func (self *OSFileInfo) Atime() glob.TimeVal {
	nsec := self.sys().LastAccessTime.Nanoseconds()
	return glob.TimeVal{
		Sec:  nsec / 1000000000,
		Nsec: nsec,
	}
}

func (self *OSFileInfo) IsLink() bool {
	return self.Mode()&os.ModeSymlink != 0
}

func (self *OSFileInfo) GetLink() (string, error) {
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

func (self *OSFileInfo) MarshalJSON() ([]byte, error) {
	result, err := json.Marshal(&struct {
		FullPath string
		Size     int64
		Mode     os.FileMode
		ModeStr  string
		ModTime  time.Time
		Sys      interface{}
		Mtime    glob.TimeVal
		Ctime    glob.TimeVal
		Atime    glob.TimeVal
	}{
		FullPath: self.FullPath(),
		Size:     self.Size(),
		Mode:     self.Mode(),
		ModeStr:  self.Mode().String(),
		ModTime:  self.ModTime(),
		Sys:      self.Sys(),
		Mtime:    self.Mtime(),
		Ctime:    self.Ctime(),
		Atime:    self.Atime(),
	})

	return result, err
}

func (u *OSFileInfo) UnmarshalJSON(data []byte) error {
	return nil
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

func GetPath(path string) string {
	expanded_path, err := registry.ExpandString(path)
	if err == nil {
		path = expanded_path
	}

	// Add a final \ to turn path into a directory path.
	path = normalize_path(path)

	// Strip leading \\ so \\c:\\windows -> c:\\windows
	return strings.TrimLeft(path, "\\")
}

type OSFileSystemAccessor struct{}

func (self OSFileSystemAccessor) New(ctx context.Context) glob.FileSystemAccessor {
	result := &OSFileSystemAccessor{}
	return result
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
			k, pres := row.Get("DeviceID")
			if pres {
				device_name, ok := k.(string)
				if ok {
					virtual_directory := glob.NewVirtualDirectoryPath(
						escape(device_name), row)
					result = append(result, virtual_directory)
				}
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
	files, err := ioutil.ReadDir(dir_path)
	if err != nil {
		// Maybe it is a symlink
		link_path := GetPath(path)
		target, err := os.Readlink(link_path)
		if err == nil {

			// Yes it is a symlink, we just recurse into
			// the target
			files, err = ioutil.ReadDir(target)
		}
	}

	for _, f := range files {
		result = append(result,
			&OSFileInfo{
				FileInfo:   f,
				_full_path: filepath.Join(path, f.Name()),
			})
	}
	return result, nil
}

func (self OSFileSystemAccessor) Open(path string) (glob.ReadSeekCloser, error) {
	// Strip leading \\ so \\c:\\windows -> c:\\windows
	path = GetPath(path)
	file, err := os.Open(path)
	return file, err
}

func (self *OSFileSystemAccessor) Lstat(path string) (glob.FileInfo, error) {
	stat, err := os.Lstat(GetPath(path))
	return &OSFileInfo{
		FileInfo:   stat,
		_full_path: path,
	}, err
}

func (self *OSFileSystemAccessor) GetRoot(path string) (string, string, error) {
	return "/", path, nil
}

// We accept both / and \ as a path separator
func (self *OSFileSystemAccessor) PathSplit() *regexp.Regexp {
	return regexp.MustCompile("[\\\\/]")
}

func (self *OSFileSystemAccessor) PathSep() string {
	return "\\"
}

// Glob sends us paths in normal form which we need to convert to
// windows form. Normal form uses / instead of \ and always has a
// leading /.
func normalize_path(path string) string {
	path = filepath.Clean(strings.Replace(path, "/", "\\", -1))
	path = strings.TrimLeft(path, "\\")
	if path == "." {
		return ""
	}
	return path
}

func init() {
	glob.Register("file", &OSFileSystemAccessor{})
}
