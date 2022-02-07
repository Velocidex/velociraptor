package zip

import (
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/third_party/zip"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	mu sync.Mutex
)

// The ZipFileSystemAccessor is cached in a singleton cached in the
// root scope. We keep a list of most recently used caches of zip
// files for quick access.
type ZipFileSystemAccessor struct {
	fd_cache map[string]*ZipFileCache
	scope    vfilter.Scope
}

// Try to remove any file caches with no references.
func (self *ZipFileSystemAccessor) Trim() {
	mu.Lock()
	defer mu.Unlock()

	cache_size := vql_subsystem.GetIntFromRow(
		self.scope, self.scope, constants.ZIP_FILE_CACHE_SIZE)
	if cache_size == 0 {
		cache_size = 5
	}

	// Grow the cache up to max 10 elements.
	for key, fd := range self.fd_cache {
		if fd == nil {
			continue
		}

		if uint64(len(self.fd_cache)) > cache_size {
			fd.mu.Lock()
			refs := fd.refs
			fd.mu.Unlock()

			if refs == 1 {
				fd.Close()
			}
		}

		// Trim closed fd from our cache.
		if fd.IsClosed() {
			delete(self.fd_cache, key)
		}
	}
}

// Close all the items - called when root scope destroys
func (self *ZipFileSystemAccessor) CloseAll() {
	mu.Lock()
	defer mu.Unlock()

	// Close all elements
	for key, fd := range self.fd_cache {
		if fd != nil {
			fd.Close()
		}
		delete(self.fd_cache, key)
	}
}

func (self *ZipFileSystemAccessor) getCachedZipFile(cache_key string) (
	*ZipFileCache, error) {
	mu.Lock()
	zip_file_cache, pres := self.fd_cache[cache_key]
	mu.Unlock()

	// The cached value is valid and ready - return it
	if pres &&
		zip_file_cache != nil &&
		!zip_file_cache.IsClosed() {
		zip_file_cache.IncRef()
		return zip_file_cache, nil
	}

	// Store a nil in the map as a place holder, while we
	// build something.
	if !pres {
		mu.Lock()
		self.fd_cache[cache_key] = nil
		mu.Unlock()
		return nil, nil
	}

	return nil, os.ErrNotExist
}

// Returns a ZipFileCache wrapper around the zip.Reader. Be sure to
// close it when done. When the query completes, the zip file will be
// closed.
func _GetZipFile(self *ZipFileSystemAccessor,
	file_path string) (*ZipFileCache, *accessors.OSPath, error) {

	// Zip files typically use standard / path separators.
	full_path := self.ParsePath(file_path)
	pathspec := full_path.PathSpec()

	base_pathspec := accessors.PathSpec{
		DelegateAccessor: pathspec.DelegateAccessor,
		DelegatePath:     pathspec.GetDelegatePath(),
	}
	cache_key := base_pathspec.String()

	for {
		zip_file_cache, err := self.getCachedZipFile(cache_key)
		if err == nil {
			if zip_file_cache == nil {
				break
			}
			return zip_file_cache, full_path, nil
		}
		time.Sleep(time.Millisecond)
	}

	accessor, err := accessors.GetAccessor(
		pathspec.DelegateAccessor, self.scope)
	if err != nil {
		self.scope.Log("%v: did you provide a URL or PathSpec?", err)
		delete(self.fd_cache, cache_key)
		return nil, nil, err
	}

	filename := pathspec.GetDelegatePath()
	fd, err := accessor.Open(filename)
	if err != nil {
		delete(self.fd_cache, cache_key)
		return nil, nil, err
	}

	reader, ok := fd.(io.ReaderAt)
	if !ok {
		self.scope.Log("file is not seekable")
		delete(self.fd_cache, cache_key)
		return nil, nil, errors.New("file is not seekable")
	}

	stat, err := accessor.Lstat(filename)
	if err != nil {
		self.scope.Log("Lstat: %v", err)
		delete(self.fd_cache, cache_key)
		return nil, nil, err
	}

	zip_file, err := zip.NewReader(reader, stat.Size())
	if err != nil {
		self.scope.Log("zip.NewReader: %v", err)
		delete(self.fd_cache, cache_key)
		return nil, nil, err
	}

	zipAccessorCurrentOpened.Inc()

	// Initial reference of 1 will be closed on scope destructor.
	zipAccessorCurrentReferences.Inc()
	zipAccessorTotalOpened.Inc()

	zip_file_cache := &ZipFileCache{
		zip_file: zip_file,
		fd:       fd,
		id:       utils.GetId(),

		// One reference to the scope.
		refs:          1,
		zip_file_name: base_pathspec.GetDelegatePath(),
	}

	for _, i := range zip_file.File {
		// Ignore directories which are signified by a
		// trailing / and have no content.
		if strings.HasSuffix(i.Name, "/") && i.UncompressedSize64 == 0 {
			continue
		}

		// Prepare the pathspec for each zip member. In order to
		// access the members, we need to open the currect zipfile (in
		// full_path) and open i.Name as the path.
		next_pathspec := full_path.PathSpec()
		next_pathspec.Path = i.Name

		next_item := _CDLookup{
			full_path:   full_path.Parse(next_pathspec.String()),
			member_file: i,
		}
		zip_file_cache.lookup = append(zip_file_cache.lookup, next_item)
	}

	// Set the new zip cache tracker in the fd cache.
	mu.Lock()

	// Leaking a zip file from this function, increase its reference -
	// callers have to close it.
	zip_file_cache.IncRef()

	// Replace the nil in the fd_cache with the real fd cache.
	self.fd_cache[cache_key] = zip_file_cache
	mu.Unlock()

	return zip_file_cache, full_path, nil
}

func (self *ZipFileSystemAccessor) Lstat(file_path string) (
	accessors.FileInfo, error) {
	if file_path == "" {
		utils.DlvBreak()
	}

	root, full_path, err := _GetZipFile(self, file_path)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	return root.GetZipInfo(full_path)
}

func (self *ZipFileSystemAccessor) Open(filename string) (accessors.ReadSeekCloser, error) {
	// Fetch the zip file from cache again.
	zip_file_cache, full_path, err := _GetZipFile(self, filename)
	if err != nil {
		return nil, err
	}
	defer zip_file_cache.Close()

	// Get the zip member from the zip file.
	return zip_file_cache.Open(full_path)
}

func (self *ZipFileSystemAccessor) ReadDir(file_path string) ([]accessors.FileInfo, error) {
	root, full_path, err := _GetZipFile(self, file_path)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	children, err := root.GetChildren(full_path)
	if err != nil {
		return nil, err
	}

	result := []accessors.FileInfo{}
	for _, item := range children {
		result = append(result, item)
	}

	return result, nil
}

const (
	ZipFileSystemAccessorTag = "_ZipFS"
)

func (self ZipFileSystemAccessor) ParsePath(path string) *accessors.OSPath {
	return accessors.NewGenericOSPath(path)
}

func (self *ZipFileSystemAccessor) New(scope vfilter.Scope) (
	accessors.FileSystemAccessor, error) {
	result_any := vql_subsystem.CacheGet(scope, ZipFileSystemAccessorTag)
	if result_any == nil {
		// Create a new cache in the scope.
		result := &ZipFileSystemAccessor{
			fd_cache: make(map[string]*ZipFileCache),
			scope:    scope,
		}
		vql_subsystem.CacheSet(scope, ZipFileSystemAccessorTag, result)

		vql_subsystem.GetRootScope(scope).AddDestructor(func() {
			result.CloseAll()
		})
		return result, nil
	}

	res := result_any.(*ZipFileSystemAccessor)
	res.Trim()

	return res, nil
}
