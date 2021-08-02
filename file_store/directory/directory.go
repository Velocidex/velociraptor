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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	openCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "file_store_open",
		Help: "Total number of filestore open operations.",
	})

	listCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "file_store_list",
		Help: "Total number of filestore list children operations.",
	})
)

const (
	// On windows all file paths must be prefixed by this to
	// support long paths.
	WINDOWS_LFN_PREFIX = "\\\\?\\"
)

type DirectoryFileWriter struct {
	Fd *os.File
}

func (self *DirectoryFileWriter) Size() (int64, error) {
	return self.Fd.Seek(0, os.SEEK_END)
}

func (self *DirectoryFileWriter) Write(data []byte) (int, error) {
	_, err := self.Fd.Seek(0, os.SEEK_END)
	if err != nil {
		return 0, err
	}

	return self.Fd.Write(data)
}

func (self *DirectoryFileWriter) Truncate() error {
	return self.Fd.Truncate(0)
}

func (self *DirectoryFileWriter) Close() error {
	return self.Fd.Close()
}

type DirectoryFileStore struct {
	config_obj *config_proto.Config
}

func NewDirectoryFileStore(config_obj *config_proto.Config) *DirectoryFileStore {
	return &DirectoryFileStore{config_obj}
}

func (self *DirectoryFileStore) Move(src, dest api.PathSpec) error {
	src_path := src.AsFilestoreFilename(self.config_obj)
	dest_path := dest.AsFilestoreFilename(self.config_obj)

	return os.Rename(src_path, dest_path)
}

func (self *DirectoryFileStore) ListDirectory(dirname api.PathSpec) (
	[]os.FileInfo, error) {

	listCounter.Inc()

	file_path := dirname.AsFilestoreDirectory(self.config_obj)
	files, err := utils.ReadDir(file_path)
	if err != nil {
		return nil, err
	}

	var result []os.FileInfo
	for _, fileinfo := range files {
		// Each file from the filesystem will be potentially
		// encoded.
		result = append(result, api.NewFileStoreFileInfo(
			self.config_obj,
			dirname.AddChild(
				utils.UnsanitizeComponent(fileinfo.Name())),
			fileinfo))
	}

	return result, nil
}

func (self *DirectoryFileStore) ReadFile(
	filename api.PathSpec) (api.FileReader, error) {
	file_path := filename.AsFilestoreFilename(self.config_obj)
	openCounter.Inc()
	file, err := os.Open(file_path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &api.FileAdapter{
		File:     file,
		FullPath: filename,
	}, nil
}

func (self *DirectoryFileStore) StatFile(
	filename api.PathSpec) (os.FileInfo, error) {
	file_path := filename.AsFilestoreFilename(self.config_obj)
	file, err := os.Stat(file_path)
	if err != nil {
		return nil, err
	}

	return &api.FileStoreFileInfo{
		FileInfo: file,
	}, nil
}

func (self *DirectoryFileStore) WriteFile(
	filename api.PathSpec) (api.FileWriter, error) {
	file_path := filename.AsFilestoreFilename(self.config_obj)
	err := os.MkdirAll(filepath.Dir(file_path), 0700)
	if err != nil {
		logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
		logger.Error("Can not create dir: %v", err)
		return nil, err
	}

	openCounter.Inc()
	file, err := os.OpenFile(file_path, os.O_RDWR|os.O_CREATE, 0700)
	if err != nil {
		logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
		logger.Error("Unable to open file %v: %v", file_path, err)

		return nil, errors.WithStack(err)
	}

	return &DirectoryFileWriter{file}, nil
}

func (self *DirectoryFileStore) Delete(filename api.PathSpec) error {
	file_path := filename.AsFilestoreFilename(self.config_obj)
	return os.Remove(file_path)
}

func (self *DirectoryFileStore) Walk(root api.PathSpec, walkFn api.WalkFunc) error {
	// Walking a non-existant directory just returns no results.
	children, err := self.ListDirectory(root)
	if err != nil {
		return nil
	}

	for _, child := range children {
		child_spec := root.AddChild(child.Name())
		if child.IsDir() {
			err = self.Walk(child_spec, walkFn)
			if err != nil {
				return err
			}
			continue
		}

		if strings.HasSuffix(child.Name(), ".db") {
			continue
		}

		walkFn(child_spec, child)
	}
	return nil
}
