package file_store_file_info

import (
	"errors"
	"os"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
)

func NewFileStoreFileInfoWithOSPath(
	config_obj *config_proto.Config,
	ospath *accessors.OSPath,
	fullpath api.FSPathSpec,
	info os.FileInfo) *FileStoreFileInfo {
	return &FileStoreFileInfo{
		FileInfo:   info,
		ospath:     ospath,
		fullpath:   fullpath,
		config_obj: config_obj,
	}
}

func NewFileStoreFileInfo(
	config_obj *config_proto.Config,
	fullpath api.FSPathSpec,
	info os.FileInfo) *FileStoreFileInfo {

	// Create an OSPath to represent the abstract filestore path.
	// Restore the file extension from the filestore abstract
	// pathspec.
	components := utils.CopySlice(fullpath.Components())
	if len(components) > 0 {
		last_idx := len(components) - 1
		components[last_idx] += api.GetExtensionForFilestore(fullpath)
	}
	ospath := accessors.MustNewFileStorePath("fs:").Append(components...)

	return &FileStoreFileInfo{
		config_obj: config_obj,
		FileInfo:   info,
		fullpath:   fullpath,
		ospath:     ospath,
	}
}

type FileStoreFileInfo struct {
	os.FileInfo
	ospath     *accessors.OSPath
	fullpath   api.FSPathSpec
	config_obj *config_proto.Config
	Data_      *ordereddict.Dict

	SizeOverride_ int64
}

func (self FileStoreFileInfo) Size() int64 {
	if self.SizeOverride_ == 0 {
		return self.FileInfo.Size()
	}
	return self.SizeOverride_
}

// We return multiple files as the base (for example the json file and
// the index both have the same basename)
func (self FileStoreFileInfo) Name() string {
	return self.fullpath.Base()
}

// This reports the unique basename
func (self FileStoreFileInfo) UniqueName() string {
	return self.ospath.Basename()
}

func (self *FileStoreFileInfo) Data() *ordereddict.Dict {
	if self.Data_ == nil {
		return ordereddict.NewDict()
	}

	return self.Data_
}

// The FullPath contains the full URL to access the filestore.
func (self *FileStoreFileInfo) FullPath() string {
	return self.ospath.String()
}

func (self *FileStoreFileInfo) OSPath() *accessors.OSPath {
	return self.ospath
}

func (self *FileStoreFileInfo) PathSpec() api.FSPathSpec {
	return self.fullpath
}

func (self *FileStoreFileInfo) Btime() time.Time {
	return time.Time{}
}

func (self *FileStoreFileInfo) Mtime() time.Time {
	return self.FileInfo.ModTime()
}

func (self *FileStoreFileInfo) Ctime() time.Time {
	return time.Time{}
}

func (self *FileStoreFileInfo) Atime() time.Time {
	return time.Time{}
}

func (self *FileStoreFileInfo) IsLink() bool {
	return self.Mode()&os.ModeSymlink != 0
}

// Filestores do not implementat links
func (self *FileStoreFileInfo) GetLink() (*accessors.OSPath, error) {
	return nil, errors.New("Not implemented")
}

func (self *FileStoreFileInfo) MarshalJSON() ([]byte, error) {
	result, err := json.Marshal(&struct {
		FullPath string
		Size     int64
		Mode     os.FileMode
		ModeStr  string
		ModTime  time.Time
		Btime    time.Time
		Mtime    time.Time
		Ctime    time.Time
		Atime    time.Time
	}{
		FullPath: self.FullPath(),
		Size:     self.Size(),
		Mode:     self.Mode(),
		ModeStr:  self.Mode().String(),
		ModTime:  self.ModTime(),
		Btime:    self.Btime(),
		Mtime:    self.Mtime(),
		Ctime:    self.Ctime(),
		Atime:    self.Atime(),
	})

	return result, err
}

func (self *FileStoreFileInfo) UnmarshalJSON(data []byte) error {
	return nil
}
