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
// A Raw NTFS accessor for disks.

// The NTFS accessor provides access to volumes, and Volume Shadow
// Copies through the VSS devices.

package filesystems

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

func ExtractI30List(accessor_ctx *AccessorContext,
	mft_entry *ntfs.MFT_ENTRY, path string) []glob.FileInfo {

	result := []glob.FileInfo{}
	ntfs_ctx := accessor_ctx.ntfs_ctx

	var lru_map map[string]*cacheMFT
	value, pres := accessor_ctx.path_listing.Get(path)
	if pres {
		lru_map = value.(cacheElement).children

	} else {
		lru_map = make(map[string]*cacheMFT)
		for _, record := range mft_entry.Dir(ntfs_ctx) {
			filename := record.File()
			name_type := filename.NameType().Name
			if name_type == "DOS" {
				continue
			}

			component := filename.Name()
			if component == "." || component == ".." {
				continue
			}

			mft_id := int64(record.MftReference())
			lru_map[strings.ToLower(component)] = &cacheMFT{
				mft_id:    mft_id,
				component: component,
				name_type: name_type,
			}
		}

		SetLRUMap(accessor_ctx, path, lru_map)
	}

	for _, item := range lru_map {
		full_path := path + "\\" + item.component

		result = append(result, &LazyNTFSFileInfo{
			mft_id:     item.mft_id,
			ntfs_ctx:   ntfs_ctx,
			name:       item.component,
			nameType:   item.name_type,
			_full_path: full_path,
		})
	}

	return result
}

type LazyNTFSFileInfo struct {
	cached_info     *ntfs.FileInfo
	ntfs_ctx        *ntfs.NTFSContext
	mft_entry       *ntfs.MFT_ENTRY
	mft_id          int64
	children        []*LazyNTFSFileInfo
	listed_children bool
	name            string
	nameType        string
	_full_path      string
}

func (self *LazyNTFSFileInfo) ensureCachedInfo() {
	if self.cached_info != nil {
		return
	}

	self.cached_info = &ntfs.FileInfo{}

	mft_entry, err := self.ntfs_ctx.GetMFT(self.mft_id)
	if err != nil {
		return
	}

	self.mft_entry = mft_entry
	file_infos := ntfs.Stat(self.ntfs_ctx, self.mft_entry)
	if len(file_infos) > 0 {
		self.cached_info = file_infos[0]
	}
}

func (self *LazyNTFSFileInfo) IsDir() bool {
	self.ensureCachedInfo()
	return self.cached_info.IsDir

	return true
}

func (self *LazyNTFSFileInfo) Size() int64 {
	self.ensureCachedInfo()
	return self.cached_info.Size
}

func (self *LazyNTFSFileInfo) Data() interface{} {
	self.ensureCachedInfo()

	result := ordereddict.NewDict().
		Set("mft", self.cached_info.MFTId).
		Set("name_type", self.cached_info.NameType)
	if self.cached_info.ExtraNames != nil {
		result.Set("extra_names", self.cached_info.ExtraNames)
	}

	return result
}

func (self *LazyNTFSFileInfo) Name() string {
	return self.name
}

func (self *LazyNTFSFileInfo) Sys() interface{} {
	return self.Data()
}

func (self *LazyNTFSFileInfo) Mode() os.FileMode {
	var result os.FileMode = 0755
	if self.IsDir() {
		result |= os.ModeDir
	}
	return result
}

func (self *LazyNTFSFileInfo) ModTime() time.Time {
	self.ensureCachedInfo()
	return self.cached_info.Mtime
}

func (self *LazyNTFSFileInfo) FullPath() string {
	return self._full_path
}

func (self *LazyNTFSFileInfo) Mtime() utils.TimeVal {
	self.ensureCachedInfo()

	return utils.TimeVal{
		Sec: self.cached_info.Mtime.Unix(),
	}
}

func (self *LazyNTFSFileInfo) Ctime() utils.TimeVal {
	self.ensureCachedInfo()

	return utils.TimeVal{
		Sec: self.cached_info.Ctime.Unix(),
	}
}

func (self *LazyNTFSFileInfo) Atime() utils.TimeVal {
	self.ensureCachedInfo()

	return utils.TimeVal{
		Sec: self.cached_info.Atime.Unix(),
	}
}

// Not supported
func (self *LazyNTFSFileInfo) IsLink() bool {
	return false
}

func (self *LazyNTFSFileInfo) GetLink() (string, error) {
	return "", errors.New("Not implemented")
}

func (self *LazyNTFSFileInfo) MarshalJSON() ([]byte, error) {
	result, err := json.Marshal(&struct {
		FullPath string
		Size     int64
		Mode     os.FileMode
		ModeStr  string
		ModTime  time.Time
		Sys      interface{}
		Mtime    utils.TimeVal
		Ctime    utils.TimeVal
		Atime    utils.TimeVal
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

type LazyNTFSFileSystemAccessor struct {
	*NTFSFileSystemAccessor
}

func (self LazyNTFSFileSystemAccessor) New(scope *vfilter.Scope) (glob.FileSystemAccessor, error) {
	base, err := NTFSFileSystemAccessor{}.New(scope)
	if err != nil {
		return nil, err
	}
	return &LazyNTFSFileSystemAccessor{base.(*NTFSFileSystemAccessor)}, nil
}

func (self *LazyNTFSFileSystemAccessor) ReadDir(path string) (res []glob.FileInfo, err error) {
	defer func() {
		r := recover()
		if r != nil {
			fmt.Printf("PANIC %v\n", r)
			err, _ = r.(error)
		}
	}()

	result := []glob.FileInfo{}

	// The path must start with a valid device, otherwise we list
	// the devices.
	device, subpath, err := self.GetRoot(path)
	if err != nil {
		vss, err := discoverVSS()
		if err == nil {
			result = append(result, vss...)
		}

		logical, err := discoverLogicalDisks()
		if err == nil {
			result = append(result, logical...)
		}

		return result, nil
	}

	accessor_ctx, err := self.getNTFSContext(device)
	if err != nil {
		return nil, err
	}

	root, err := accessor_ctx.ntfs_ctx.GetMFT(5)
	if err != nil {
		return nil, err
	}

	// Open the device path from the root.
	dir, err := Open(root, accessor_ctx, device, subpath)
	if err != nil {
		return nil, err
	}

	result = ExtractI30List(accessor_ctx, dir, path)
	return result, nil
}

func (self *LazyNTFSFileSystemAccessor) Open(path string) (res glob.ReadSeekCloser, err error) {
	defer func() {
		r := recover()
		if r != nil {
			fmt.Printf("PANIC %v\n", r)
			err, _ = r.(error)
		}
	}()

	// The path must start with a valid device, otherwise we list
	// the devices.
	device, subpath, err := self.GetRoot(path)
	if err != nil {
		return nil, errors.New("Unable to open raw device")
	}

	components := self.PathSplit(subpath)

	accessor_ctx, err := self.getNTFSContext(device)
	if err != nil {
		return nil, err
	}

	ntfs_ctx := accessor_ctx.ntfs_ctx
	root, err := ntfs_ctx.GetMFT(5)
	if err != nil {
		return nil, err
	}

	// Open the device path from the root.
	mft_entry, err := Open(root, accessor_ctx, device, subpath)
	if err != nil {
		return nil, err
	}

	// Get the first data attribute.
	data_attr, err := mft_entry.GetAttribute(ntfs_ctx, 128, -1)
	if err != nil {
		return nil, err
	}

	reader, err := ntfs.OpenStream(ntfs_ctx, mft_entry,
		128, data_attr.Attribute_id())
	if err != nil {
		return nil, err
	}

	return &readAdapter{
		info: &LazyNTFSFileInfo{
			mft_id:     int64(mft_entry.Record_number()),
			ntfs_ctx:   ntfs_ctx,
			name:       components[len(components)-1],
			_full_path: path,
		},
		reader: reader,
	}, nil
}

func init() {
	glob.Register("lazy_ntfs", &LazyNTFSFileSystemAccessor{})
}
