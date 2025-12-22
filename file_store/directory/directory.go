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

	"github.com/go-errors/errors"
	"www.velocidex.com/golang/velociraptor/accessors/file_store_file_info"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/third_party/cache"
	"www.velocidex.com/golang/velociraptor/utils"
)

type DirectoryFileStore struct {
	config_obj *config_proto.Config
	db         datastore.DataStore
}

func NewDirectoryFileStore(config_obj *config_proto.Config) *DirectoryFileStore {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil
	}
	return &DirectoryFileStore{
		config_obj: config_obj,
		db:         db,
	}
}

func (self *DirectoryFileStore) Move(src, dest api.FSPathSpec) error {
	src_path := datastore.AsFilestoreFilename(self.db, self.config_obj, src)
	dest_path := datastore.AsFilestoreFilename(self.db, self.config_obj, dest)

	// Ensure the directories exist.
	err := datastore.MkdirAll(self.db, self.config_obj, dest.Dir())
	if err != nil {
		logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
		logger.Error("Can not create dir %v: %v", dest.Dir(), err)
		return err
	}

	return os.Rename(src_path, dest_path)
}

func (self *DirectoryFileStore) Close() error {
	return nil
}

func (self *DirectoryFileStore) ListDirectory(dirname api.FSPathSpec) (
	[]api.FileInfo, error) {

	defer api.InstrumentWithDelay("list", "DirectoryFileStore", dirname)()

	file_path := datastore.AsFilestoreDirectory(
		self.db, self.config_obj, dirname)
	files, err := utils.ReadDir(file_path)
	if err != nil {
		return nil, err
	}

	untyped := path_specs.IsComponentUntyped(dirname.Components())

	var result []api.FileInfo
	for _, fileinfo := range files {
		// Each file from the filesystem will be potentially
		// encoded.
		name := fileinfo.Name()

		// Eliminate the data store files
		if strings.HasSuffix(name, ".db") {
			continue
		}

		// Name may be compressed
		name = datastore.UncompressComponent(
			self.db, self.config_obj, name)

		var name_type api.PathType
		if fileinfo.IsDir() {
			name_type = api.PATH_TYPE_DATASTORE_DIRECTORY

		} else if untyped {
			name_type = api.PATH_TYPE_FILESTORE_ANY

		} else {
			name_type, name = api.GetFileStorePathTypeFromExtension(name)
		}

		result = append(result,
			file_store_file_info.NewFileStoreFileInfo(self.config_obj,
				dirname.AddUnsafeChild(name).SetType(name_type),
				fileinfo))
	}

	return result, nil
}

func isPathCompressible(path api.FSPathSpec) bool {
	switch path.Type() {
	case api.PATH_TYPE_FILESTORE_CHUNK_INDEX,
		api.PATH_TYPE_FILESTORE_JSON_INDEX,
		api.PATH_TYPE_FILESTORE_SPARSE_IDX:
		return false
	default:
		return true
	}
}

func (self *DirectoryFileStore) ReadFile(
	filename api.FSPathSpec) (api.FileReader, error) {
	file_path := datastore.AsFilestoreFilename(
		self.db, self.config_obj, filename)

	defer api.InstrumentWithDelay("open_read", "DirectoryFileStore", filename)()

	err := checkPath(file_path)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(file_path)
	if err != nil {
		return nil, errors.Wrap(err, 0)
	}
	reader := &api.FileAdapter{
		File:      file,
		PathSpec_: filename,
	}

	if !isPathCompressible(filename) {
		return reader, nil
	}

	chunk_file_path := datastore.AsFilestoreFilename(
		self.db, self.config_obj, filename.
			SetType(api.PATH_TYPE_FILESTORE_CHUNK_INDEX))

	err = checkPath(chunk_file_path)
	if err != nil {
		return nil, err
	}

	chunk_fd, err := os.Open(chunk_file_path)
	if err != nil {
		return reader, nil
	}

	return &CompressedDirectoryReader{
		chunkIndex: api.NewChunkIndex(&api.FileAdapter{
			File: chunk_fd,
		}),
		reader:     reader,
		chunkCache: cache.NewLRUCache(10),
	}, nil
}

func (self *DirectoryFileStore) StatFile(
	filename api.FSPathSpec) (api.FileInfo, error) {

	defer api.Instrument("stat", "DirectoryFileStore", filename)()

	file_path := datastore.AsFilestoreFilename(
		self.db, self.config_obj, filename)
	file, err := os.Stat(file_path)
	if err != nil {
		return nil, err
	}

	return file_store_file_info.NewFileStoreFileInfo(
		self.config_obj, filename, file), nil
}

func (self *DirectoryFileStore) WriteFile(
	filename api.FSPathSpec) (api.FileWriter, error) {
	return self.WriteFileWithCompletion(filename, utils.SyncCompleter)
}

func (self *DirectoryFileStore) WriteFileWithCompletion(
	filename api.FSPathSpec, completion func()) (api.FileWriter, error) {

	defer api.InstrumentWithDelay("open_write", "DirectoryFileStore", filename)()

	// Writes are only possible when the datastore is healthy.
	err := self.db.Healthy()
	if err != nil {
		return nil, err
	}

	err = datastore.MkdirAll(self.db, self.config_obj, filename.Dir())
	if err != nil {
		logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
		logger.Error("Can not create dir: %v", err)
		return nil, err
	}

	file_path := datastore.AsFilestoreFilename(self.db, self.config_obj, filename)
	err = checkPath(file_path)
	if err != nil {
		return nil, err
	}

	file, err := os.OpenFile(file_path, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
		logger.Error("Unable to open file %v: %v", file_path, err)

		return nil, errors.Wrap(err, 0)
	}

	var chunk_fd *os.File

	if isPathCompressible(filename) {
		chunk_file_path := datastore.AsFilestoreFilename(
			self.db, self.config_obj, filename.
				SetType(api.PATH_TYPE_FILESTORE_CHUNK_INDEX))

		err = checkPath(chunk_file_path)
		if err != nil {
			return nil, err
		}

		// If the index exists, we open it for append mode. If the
		// index does not exist, then we wait to create it on the
		// first call to WriteCompressed()
		chunk_fd, err = os.OpenFile(chunk_file_path, os.O_RDWR, 0600)
		if errors.Is(err, os.ErrNotExist) {
			// Delay chunk writer until first call to WriteCompressed()
			chunk_fd = nil
		}
	}

	return &DirectoryFileWriter{
		Fd:         file,
		ChunkFd:    chunk_fd,
		path:       filename,
		db:         self.db,
		config_obj: self.config_obj,

		completion: completion,
	}, nil
}

func (self *DirectoryFileStore) Delete(filename api.FSPathSpec) error {

	defer api.InstrumentWithDelay("delete", "DirectoryFileStore", filename)()

	file_path := datastore.AsFilestoreFilename(
		self.db, self.config_obj, filename)
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
			logger.Debug("Pruning empty directory %v", dir_name)
		}

		// Check if this is the last file in the directory.
		dir_name = filepath.Dir(dir_name)
	}

	// At least we succeeded deleting the file
	return nil
}
