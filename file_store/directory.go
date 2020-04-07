// This is an implementation of the file store based on single files
// and directories on the filesystem.

// It is very fast but can not be shared between multiple hosts since
// the filesystem must be locally accessible. A remote filesystem,
// such as NFS might work but this configuration is not tested nor
// supported.

package file_store

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
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
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

func (self *DirectoryFileStore) ListDirectory(dirname string) (
	[]os.FileInfo, error) {

	listCounter.Inc()

	file_path := self.FilenameToFileStorePath(dirname)
	files, err := utils.ReadDir(file_path)
	if err != nil {
		return nil, err
	}

	var result []os.FileInfo
	for _, fileinfo := range files {
		result = append(result, &DirectoryFileStoreFileInfo{
			fileinfo,
			utils.PathJoin(dirname, fileinfo.Name(), "/"),
			nil})
	}

	return result, nil
}

func getCompressed(filename string) (FileReader, error) {
	fd, err := os.Open(filename)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	zr, err := gzip.NewReader(fd)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &GzipReader{zr, fd, filename}, nil
}

func (self *DirectoryFileStore) ReadFile(filename string) (FileReader, error) {
	file_path := self.FilenameToFileStorePath(filename)
	if strings.HasSuffix(".gz", file_path) {
		return getCompressed(file_path)
	}

	openCounter.Inc()
	file, err := os.Open(file_path)
	if os.IsNotExist(err) {
		return getCompressed(file_path + ".gz")
	}

	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &FileAdapter{file, filename}, nil
}

func (self *DirectoryFileStore) StatFile(filename string) (os.FileInfo, error) {
	file_path := self.FilenameToFileStorePath(filename)
	file, err := os.Stat(file_path)
	if err != nil {
		return nil, err
	}

	return &DirectoryFileStoreFileInfo{file, filename, nil}, nil
}

func (self *DirectoryFileStore) WriteFile(filename string) (FileWriter, error) {
	file_path := self.FilenameToFileStorePath(filename)
	err := os.MkdirAll(filepath.Dir(file_path), 0700)
	if err != nil {
		logging.GetLogger(self.config_obj,
			&logging.FrontendComponent).Error(
			"Can not create dir", err)
		return nil, err
	}

	openCounter.Inc()
	file, err := os.OpenFile(file_path, os.O_RDWR|os.O_CREATE, 0700)
	if err != nil {
		logging.GetLogger(self.config_obj, &logging.FrontendComponent).Error(
			"Unable to open file "+file_path, err)
		return nil, errors.WithStack(err)
	}

	return &DirectoryFileWriter{file}, nil
}

func (self *DirectoryFileStore) Delete(filename string) error {
	file_path := self.FilenameToFileStorePath(filename)
	return os.Remove(file_path)
}

// In the below:
// Filename: is an abstract filename to be represented in the file store.
// FileStorePath: An actual path to store the file on the filesystem.
//
// On windows, the FileStorePath always includes the LFN prefix.
func (self *DirectoryFileStore) FilenameToFileStorePath(filename string) string {
	components := []string{self.config_obj.Datastore.FilestoreDirectory}
	for _, component := range utils.SplitComponents(filename) {
		components = append(components,
			string(datastore.SanitizeString(component)))
	}

	result := filepath.Join(components...)
	if runtime.GOOS == "windows" {
		return WINDOWS_LFN_PREFIX + result
	}
	return result
}

// Converts from a physical path on disk to a normalized filestore path.
func (self *DirectoryFileStore) FileStorePathToFilename(filename string) (
	string, error) {

	if runtime.GOOS == "windows" {
		filename = strings.TrimPrefix(filename, WINDOWS_LFN_PREFIX)
	}
	filename = strings.TrimPrefix(filename,
		self.config_obj.Datastore.FilestoreDirectory)

	components := []string{}
	for _, component := range strings.Split(
		filename,
		string(os.PathSeparator)) {
		components = append(components,
			string(datastore.UnsanitizeComponent(component)))
	}

	result := filepath.Join(components...)
	return result, nil
}

func (self *DirectoryFileStore) Walk(root string, walkFn filepath.WalkFunc) error {
	path := self.FilenameToFileStorePath(root)
	return filepath.Walk(path,
		func(path string, info os.FileInfo, err error) error {
			filename, err_1 := self.FileStorePathToFilename(path)
			if err_1 != nil {
				return err_1
			}
			return walkFn(filename,
				&DirectoryFileStoreFileInfo{info, path, nil}, err)
		})
}
