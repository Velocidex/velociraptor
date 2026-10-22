package file_store

// This implements a filesystem accessor which can be used to access
// the generic filestore. This allows us to run globs on the file
// store regardless of the specific filestore implementation.
import (
	"errors"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/file_store_file_info"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/timelines"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/files"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type FileStoreFileSystemAccessor struct {
	file_store api.FileStore
	config_obj *config_proto.Config

	sparse bool
}

func NewFileStoreFileSystemAccessor(
	config_obj *config_proto.Config) *FileStoreFileSystemAccessor {
	return &FileStoreFileSystemAccessor{
		file_store: file_store.GetFileStore(config_obj),
		config_obj: config_obj,
	}
}

type SparseFileStoreFileSystemAccessor struct {
	FileStoreFileSystemAccessor
}

func (self SparseFileStoreFileSystemAccessor) Describe() *accessors.AccessorDescriptor {
	return &accessors.AccessorDescriptor{
		Name: "fs_sparse",
		Description: `Provide access to the server's filestore and datastore.

This accessor expands sparse files. Reading from a sparse region will result in zeros being returned.
`,
		Permissions: []acls.ACL_PERMISSION{acls.SERVER_ADMIN},
	}
}

func NewSparseFileStoreFileSystemAccessor(
	config_obj *config_proto.Config) *SparseFileStoreFileSystemAccessor {
	return &SparseFileStoreFileSystemAccessor{
		FileStoreFileSystemAccessor: FileStoreFileSystemAccessor{
			file_store: file_store.GetFileStore(config_obj),
			config_obj: config_obj,
			sparse:     true,
		}}
}

func (self FileStoreFileSystemAccessor) Describe() *accessors.AccessorDescriptor {
	return &accessors.AccessorDescriptor{
		Name: "fs",
		Description: `Provide access to the server's filestore and datastore.

Many VQL plugins produce references to files stored on the server. This accessor can be used to open those files and read them. Typically references to filestore or datastore files have the "fs:" or "ds:" prefix.
`,
		Permissions: []acls.ACL_PERMISSION{acls.SERVER_ADMIN},
	}
}

func (self FileStoreFileSystemAccessor) New(
	scope vfilter.Scope) (accessors.FileSystemAccessor, error) {
	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		return &FileStoreFileSystemAccessor{
			file_store: self.file_store,
			config_obj: self.config_obj,
			sparse:     self.sparse,
		}, nil
	}

	return &FileStoreFileSystemAccessor{
		file_store: file_store.GetFileStore(config_obj),
		config_obj: config_obj,
		sparse:     self.sparse,
	}, nil
}

func (self FileStoreFileSystemAccessor) Lstat(filename string) (
	accessors.FileInfo, error) {
	full_path, err := self.ParsePath(filename)
	if err != nil {
		return nil, err
	}

	return self.LstatWithOSPath(full_path)
}

func (self FileStoreFileSystemAccessor) LstatWithOSPath(
	filename *accessors.OSPath) (
	accessors.FileInfo, error) {

	fullpath := path_specs.FromGenericComponentList(filename.Components)
	err := IsFileAccessible(fullpath)
	if err != nil {
		return nil, err
	}

	lstat, err := self.file_store.StatFile(fullpath)
	if err != nil {
		// If it didn't work, we try case insensitive open
		corrected_path, err := getCorrectCase(self.file_store, fullpath)
		if err != nil {
			return nil, err
		}
		lstat, err = self.file_store.StatFile(corrected_path)
		if err != nil {
			return nil, err
		}
	}

	stat := file_store_file_info.NewFileStoreFileInfoWithOSPath(
		self.config_obj, filename, fullpath, lstat)

	if self.sparse {
		index, err := getIndex(self.config_obj, fullpath)
		if err != nil {
			return stat, nil
		}

		if len(index.Ranges) > 0 {
			run := index.Ranges[len(index.Ranges)-1]
			stat.SizeOverride_ = run.OriginalOffset + run.FileLength
		}
	}

	return stat, nil
}

func (self FileStoreFileSystemAccessor) ParsePath(path string) (
	*accessors.OSPath, error) {
	return accessors.NewFileStorePath(path)
}

func (self FileStoreFileSystemAccessor) ReadDir(filename string) (
	[]accessors.FileInfo, error) {
	full_path, err := self.ParsePath(filename)
	if err != nil {
		return nil, err
	}

	return self.ReadDirWithOSPath(full_path)
}

func (self FileStoreFileSystemAccessor) ReadDirWithOSPath(
	filename *accessors.OSPath) (
	[]accessors.FileInfo, error) {

	fullpath := path_specs.FromGenericComponentList(filename.Components)
	err := IsFileAccessible(fullpath)
	if err != nil {
		return nil, err
	}

	files, err := self.file_store.ListDirectory(fullpath)
	if err != nil {
		// If it didn't work, we try case insensitive
		corrected_path, err := getCorrectCase(self.file_store, fullpath)
		if err != nil {
			return nil, err
		}

		files, err = self.file_store.ListDirectory(corrected_path)
		if err != nil {
			return nil, err
		}
	}

	var result []accessors.FileInfo
	for _, f := range files {
		child_path := f.PathSpec()
		err := IsFileAccessible(child_path)
		if err != nil {
			continue
		}

		child := file_store_file_info.NewFileStoreFileInfo(
			self.config_obj, f.PathSpec(), f)

		if !f.IsDir() {
			// Do this lazily
			child.DataCB_ = fetchExtraData
		}

		result = append(result, child)
	}

	return result, nil
}

func fetchExtraData(config_obj *config_proto.Config,
	fullpath api.FSPathSpec) *ordereddict.Dict {
	desc := paths.DescribePath(fullpath)
	res := ordereddict.NewDict().
		Set("Type", desc.Type).
		Set("PathType", string(desc.PathType))

	file_store_factory := file_store.GetFileStore(config_obj)

	// Decorate the data with additional information depending on what
	// the file actually is.
	switch desc.PathType {
	case paths.RESULT_SET:
		rs_reader, err := result_sets.NewResultSetReader(
			file_store_factory, fullpath)
		if err == nil {
			stats := rs_reader.Stats()
			res.Set("Rows", stats.Rows).
				Set("ModTime", stats.ModTime).
				Set("Size", stats.UncompressedSize).
				Set("CompressedSize", stats.CompressedSize)
		}

	case paths.BULK_DATA:
		stats, err := file_store_factory.StatFile(fullpath)
		if err == nil {
			compressed_size := stats.Size()
			compressed_fd, ok := stats.(api.CompressedFileInfo)
			if ok {
				compressed_size = compressed_fd.RawSize()
			}

			res.Set("Size", stats.Size()).
				Set("CompressedSize", compressed_size).
				Set("LastModified", stats.ModTime()).
				Set("PathType", fullpath.Type().String())
		}

	case paths.DATASTORE_OBJECT:
		db, err := datastore.GetDB(config_obj)
		if err == nil {
			// Check if the file is actually there
			stats, err := db.Stat(
				config_obj, fullpath.AsDatastorePath())
			if err == nil {
				res.Set("Size", stats.Size).
					Set("Mtime", stats.ModTime())
			}
		}

	case paths.TIMED_RESULT_SET:
		path_manager := paths.NewTimelinePathManager("", fullpath)
		reader_factory := timelines.TimelineReader{}
		reader, err := reader_factory.New(
			config_obj, timelines.UnitTransformer, path_manager)
		if err == nil {
			stats := reader.Stat()
			res.Set("StartTime", time.Unix(0, stats.StartTime)).
				Set("EndTime", time.Unix(0, stats.EndTime)).
				Set("Size", stats.Size).
				Set("Rows", stats.Rows).
				Set("LastModified", time.Unix(stats.LastModified, 0))
		}
	}

	// Extract additional metadata
	if desc.Decorator != nil {
		res.MergeFrom(desc.Decorator(fullpath))
	}
	return res
}

func (self FileStoreFileSystemAccessor) Open(filename string) (
	accessors.ReadSeekCloser, error) {
	full_path, err := self.ParsePath(filename)
	if err != nil {
		return nil, err
	}

	return self.OpenWithOSPath(full_path)
}

func (self FileStoreFileSystemAccessor) OpenWithOSPath(filename *accessors.OSPath) (
	accessors.ReadSeekCloser, error) {

	if len(filename.Components) == 0 {
		return nil, errors.New("Invalid path")
	}

	var fullpath api.FSPathSpec

	// It is a data store path
	if filename.PathSpec().DelegatePath == "ds:" {
		ds_path := getDSPathSpec(filename)
		fullpath = ds_path.AsFilestorePath()
		switch ds_path.Type() {
		case api.PATH_TYPE_DATASTORE_JSON:
			fullpath = fullpath.SetType(api.PATH_TYPE_FILESTORE_DB_JSON)

		case api.PATH_TYPE_DATASTORE_PROTO:
			fullpath = fullpath.SetType(api.PATH_TYPE_FILESTORE_DB)
		}

	} else {
		fullpath = path_specs.FromGenericComponentList(filename.Components)
	}

	err := IsFileAccessible(fullpath)
	if err != nil {
		return nil, err
	}

	file, err := self.openFile(fullpath)
	if err != nil {
		// Try to open the old protobuf style files as a fallback.
		if fullpath.Type() == api.PATH_TYPE_FILESTORE_DB_JSON {
			file, err = self.openFile(fullpath.SetType(api.PATH_TYPE_FILESTORE_DB))
		}

		if err != nil {
			// If it didn't work, we try case insensitive open
			corrected_path, err := getCorrectCase(self.file_store, fullpath)
			if err != nil {
				return nil, err
			}

			file, err = self.openFile(corrected_path)
			if err != nil {
				return nil, err
			}
		}
	}

	return file, nil
}

func (self FileStoreFileSystemAccessor) openFile(filename api.FSPathSpec) (
	accessors.ReadSeekCloser, error) {
	file, err := self.file_store.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	key := filename.AsClientPath()
	files.Add(key)

	if !self.sparse {
		return file, err
	}

	index, err := getIndex(self.config_obj, filename)
	if err != nil {
		return file, nil
	}

	// Wrap the file with the index.
	reader_at, err := utils.NewPagedReader(&utils.RangedReader{
		ReaderAt: utils.MakeReaderAtter(file),
		Index:    index,
	}, 0x1000, 100)
	if err != nil {
		return nil, err
	}

	return &ReaderWrapper{
		ReadSeekCloser: utils.NewReadSeekReaderAdapter(reader_at, func() {
			files.Remove(key)
		}),
		Index: index,
	}, nil
}

type ReaderWrapper struct {
	accessors.ReadSeekCloser
	Index *actions_proto.Index
}

// ReaderWrapper provides a Ranges() method so consumers can see
// the sparse regions.
func (self *ReaderWrapper) Ranges() (res []uploads.Range) {
	for _, run := range self.Index.Ranges {
		res = append(res, uploads.Range{
			Offset:   run.OriginalOffset,
			Length:   run.Length,
			IsSparse: run.FileLength == 0,
		})
	}

	return res
}

func getDSPathSpec(filename *accessors.OSPath) api.DSPathSpec {
	result := path_specs.NewUnsafeDatastorePath(filename.Components...)
	if len(filename.Components) > 0 {
		last := len(filename.Components) - 1
		name_type, name := api.GetDataStorePathTypeFromExtension(
			filename.Components[last])
		filename.Components[last] = name
		return result.SetType(name_type)
	}
	return result
}

// Load the index from the filestore if it is there.
func getIndex(config_obj *config_proto.Config,
	vfs_path api.FSPathSpec) (*actions_proto.Index, error) {
	index := &actions_proto.Index{}

	file_store_factory := file_store.GetFileStore(config_obj)
	fd, err := file_store_factory.ReadFile(
		vfs_path.SetType(api.PATH_TYPE_FILESTORE_SPARSE_IDX))
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	data, err := utils.ReadAllWithLimit(fd, constants.MAX_MEMORY)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, &index)
	if err != nil {
		return nil, err
	}

	return index, nil
}
