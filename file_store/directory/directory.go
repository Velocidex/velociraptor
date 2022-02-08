// This is an implementation of the file store based on single files
// and directories on the filesystem.

// It is very fast but can not be shared between multiple hosts since
// the filesystem must be locally accessible. A remote filesystem,
// such as NFS might work but this configuration is not tested nor
// supported.

package directory

/*
  This file store implementation stores files on disk. All of these
  functions receive serialized Velociraptor's VFS paths.

  Velociraptor paths are a sequence of string components. When the VFS
  path is serialized, we join the components using the path separator
  (by default /) . If the component contains path separators, they
  will be escaped (see utils/path.go).

  There is a direct mapping between VFS paths and filenames on
  disk. This mapping is reversible and supports correct round
  tripping.

  Use FilenameToFileStorePath() to convert from a serialized VFS path to a disk path

  Calling any of the Filestore methods (ReadDir, Open, Lstat) will
  always return VFS paths.
*/

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	file_store_accessor "www.velocidex.com/golang/velociraptor/accessors/file_store"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

const (
	// On windows all file paths must be prefixed by this to
	// support long paths.
	WINDOWS_LFN_PREFIX = "\\\\?\\"
)

type DirectoryFileWriter struct {
	Fd         *os.File
	completion func()
}

func (self *DirectoryFileWriter) Size() (int64, error) {
	return self.Fd.Seek(0, os.SEEK_END)
}

func (self *DirectoryFileWriter) Write(data []byte) (int, error) {

	defer api.InstrumentWithDelay("write", "DirectoryFileWriter", nil)()

	_, err := self.Fd.Seek(0, os.SEEK_END)
	if err != nil {
		return 0, err
	}

	return self.Fd.Write(data)
}

func (self *DirectoryFileWriter) Truncate() error {
	return self.Fd.Truncate(0)
}

func (self *DirectoryFileWriter) Flush() error { return nil }

func (self *DirectoryFileWriter) Close() error {
	err := self.Fd.Close()

	// DirectoryFileWriter is synchronous... complete on Close()
	if self.completion != nil &&
		!utils.CompareFuncs(self.completion, utils.SyncCompleter) {
		self.completion()
	}
	return err
}

type DirectoryFileStore struct {
	config_obj *config_proto.Config
}

func NewDirectoryFileStore(config_obj *config_proto.Config) *DirectoryFileStore {
	return &DirectoryFileStore{config_obj}
}

func (self *DirectoryFileStore) Move(src, dest api.FSPathSpec) error {
	src_path := src.AsFilestoreFilename(self.config_obj)
	dest_path := dest.AsFilestoreFilename(self.config_obj)

	return os.Rename(src_path, dest_path)
}

func (self *DirectoryFileStore) Close() error {
	return nil
}

func (self *DirectoryFileStore) ListDirectory(dirname api.FSPathSpec) (
	[]api.FileInfo, error) {

	defer api.InstrumentWithDelay("list", "DirectoryFileStore", dirname)()

	file_path := dirname.AsFilestoreDirectory(self.config_obj)
	files, err := utils.ReadDir(file_path)
	if err != nil {
		return nil, err
	}

	var result []api.FileInfo
	for _, fileinfo := range files {
		// Each file from the filesystem will be potentially
		// encoded.
		name := fileinfo.Name()

		// Eliminate the data store files
		if strings.HasSuffix(name, ".db") {
			continue
		}

		name_type, name := api.GetFileStorePathTypeFromExtension(name)
		result = append(result, file_store_accessor.NewFileStoreFileInfo(
			self.config_obj,
			dirname.AddChild(
				utils.UnsanitizeComponent(name)).
				SetType(name_type),
			fileinfo))
	}

	return result, nil
}

func (self *DirectoryFileStore) ReadFile(
	filename api.FSPathSpec) (api.FileReader, error) {
	file_path := filename.AsFilestoreFilename(self.config_obj)

	defer api.InstrumentWithDelay("open_read", "DirectoryFileStore", filename)()

	file, err := os.Open(file_path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &api.FileAdapter{
		File:      file,
		PathSpec_: filename,
	}, nil
}

func (self *DirectoryFileStore) StatFile(
	filename api.FSPathSpec) (api.FileInfo, error) {

	defer api.Instrument("stat", "DirectoryFileStore", filename)()

	file_path := filename.AsFilestoreFilename(self.config_obj)
	file, err := os.Stat(file_path)
	if err != nil {
		return nil, err
	}

	return &file_store_accessor.FileStoreFileInfo{
		FileInfo: file,
	}, nil
}

func (self *DirectoryFileStore) WriteFile(
	filename api.FSPathSpec) (api.FileWriter, error) {
	if strings.Contains(filename.AsClientPath(), "Generic.Client.Stats") {
		utils.DlvBreak()
	}

	return self.WriteFileWithCompletion(filename, nil)
}

func (self *DirectoryFileStore) WriteFileWithCompletion(
	filename api.FSPathSpec, completion func()) (api.FileWriter, error) {

	defer api.InstrumentWithDelay("open_write", "DirectoryFileStore", filename)()

	file_path := filename.AsFilestoreFilename(self.config_obj)
	err := os.MkdirAll(filepath.Dir(file_path), 0700)
	if err != nil {
		logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
		logger.Error("Can not create dir: %v", err)
		return nil, err
	}

	file, err := os.OpenFile(file_path, os.O_RDWR|os.O_CREATE, 0700)
	if err != nil {
		logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
		logger.Error("Unable to open file %v: %v", file_path, err)

		return nil, errors.WithStack(err)
	}

	return &DirectoryFileWriter{
		Fd:         file,
		completion: completion,
	}, nil
}

func (self *DirectoryFileStore) Delete(filename api.FSPathSpec) error {

	defer api.InstrumentWithDelay("delete", "DirectoryFileStore", filename)()

	file_path := filename.AsFilestoreFilename(self.config_obj)
	err := os.Remove(file_path)
	if err != nil {
		return err
	}

	dir_name := filepath.Dir(file_path)

	// Exit as soon as directory is not empty
	for err == nil {
		// Remove all empty leading directories
		err = os.Remove(dir_name)
		if err == nil {
			logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
			logger.Debug("Prunning empty directory %v", dir_name)
		}

		// Check if this is the last file in the directory.
		dir_name = filepath.Dir(dir_name)
	}

	// At least we succeeded deleting the file
	return nil
}
