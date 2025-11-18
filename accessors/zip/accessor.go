package zip

import (
	"strings"
	"sync"
	"time"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/third_party/zip"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

const (
	ZipFileSystemAccessorTag       = "_ZipFS"
	ZipFileNoCaseSystemAccessorTag = "_ZipFSNoCase"
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
	nocase   bool
}

func (self *ZipFileSystemAccessor) Copy(
	scope vfilter.Scope) *ZipFileSystemAccessor {
	mu.Lock()
	defer mu.Unlock()

	return &ZipFileSystemAccessor{
		fd_cache: self.fd_cache,
		nocase:   self.nocase,
		scope:    scope,
	}
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

	zip_file_cache, pres := self.fd_cache[cache_key]

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
		self.fd_cache[cache_key] = nil
		return nil, nil
	}

	return nil, utils.NotFoundError
}

// Returns a ZipFileCache wrapper around the zip.Reader. Be sure to
// close it when done. When the query completes, the zip file will be
// closed.
func _GetZipFile(self *ZipFileSystemAccessor,
	full_path *accessors.OSPath) (result *ZipFileCache, err error) {

	pathspec := full_path.PathSpec()

	base_pathspec := accessors.PathSpec{
		DelegateAccessor: pathspec.DelegateAccessor,
		DelegatePath:     pathspec.GetDelegatePath(),
	}
	cache_key := base_pathspec.String() + full_path.DescribeType()

	for {
		mu.Lock()
		zip_file_cache, err := self.getCachedZipFile(cache_key)
		if err == nil {
			// This means the zip file cache needs to be built - we
			// are still holding the lock and will release it below.
			if zip_file_cache == nil {
				mu.Unlock()
				break
			}
			mu.Unlock()
			return zip_file_cache, nil
		}
		mu.Unlock()
		time.Sleep(time.Millisecond)
	}

	defer func() {
		if err != nil {
			mu.Lock()
			defer mu.Unlock()
			delete(self.fd_cache, cache_key)
		}
	}()

	accessor, err := accessors.GetAccessor(
		pathspec.DelegateAccessor, self.scope)
	if err != nil {
		self.scope.Log("ZipFileSystemAccessor: %v", err)
		return nil, err
	}

	filename := pathspec.GetDelegatePath()
	fd, err := accessor.Open(filename)
	if err != nil {
		return nil, err
	}

	stat, err := accessor.Lstat(filename)
	if err != nil {
		self.scope.Log("ZipFileSystemAccessor: %v", err)
		return nil, err
	}

	reader_atter := utils.MakeReaderAtter(fd)
	zip_file, err := zip.NewReader(reader_atter, stat.Size())
	if err != nil {
		return nil, err
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
		scope:         self.scope,
	}

	for _, i := range zip_file.File {
		if strings.HasPrefix(i.Name, "{") {
			continue
		}

		// Ignore directories which are signified by a
		// trailing / and have no content.
		if strings.HasSuffix(i.Name, "/") && i.UncompressedSize64 == 0 {
			continue
		}

		// Prepare the pathspec for each zip member. In order to
		// access the members, we need to open the current zipfile (in
		// full_path) and open i.Name as the path.

		// So if the zip has has a pathspec like:
		// {DelegateAccessor: "auto", DelegatePath: "path/to/zip"}

		// We need to parse the zip members (unescaping as needed into
		// components), and append those components to the zip
		// pathspec to get at the member pathspec.
		//
		// {DelegateAccessor: "auto", DelegatePath: "path/to/zip",
		//  Path: "zip_escaped_component_list"}
		next_path := full_path.Copy()

		// Parse the i.Name as an encoded zip file. Some Zip accessors
		// (e.g. the collector accessor) use a special path
		// manipulator that allows any path to be represented in a zip
		// file by encoding unrepresentable characters.
		zip_path, err := full_path.Parse(i.Name)
		if err != nil {
			continue
		}
		next_path.Components = zip_path.Components

		next_item := _CDLookup{
			full_path:   next_path,
			member_file: i,
		}
		zip_file_cache.lookup = append(zip_file_cache.lookup, next_item)
	}

	// Set the new zip cache tracker in the fd cache.
	tracker.Inc(zip_file_cache.zip_file_name)

	// Leaking a zip file from this function, increase its reference -
	// callers have to close it.
	zip_file_cache.IncRef()

	// Replace the nil in the fd_cache with the real fd cache.
	mu.Lock()
	self.fd_cache[cache_key] = zip_file_cache
	mu.Unlock()

	return zip_file_cache, nil
}

func (self *ZipFileSystemAccessor) Lstat(file_path string) (
	accessors.FileInfo, error) {
	full_path, err := self.ParsePath(file_path)
	if err != nil {
		return nil, err
	}

	return self.LstatWithOSPath(full_path)
}

func (self *ZipFileSystemAccessor) LstatWithOSPath(
	full_path *accessors.OSPath) (accessors.FileInfo, error) {

	root, err := _GetZipFile(self, full_path)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	return root.GetZipInfo(full_path, self.nocase)
}

func (self *ZipFileSystemAccessor) Open(
	filename string) (accessors.ReadSeekCloser, error) {

	full_path, err := self.ParsePath(filename)
	if err != nil {
		return nil, err
	}

	return self.OpenWithOSPath(full_path)
}

func (self *ZipFileSystemAccessor) OpenWithOSPath(
	full_path *accessors.OSPath) (accessors.ReadSeekCloser, error) {

	// Fetch the zip file from cache again.
	zip_file_cache, err := _GetZipFile(self, full_path)
	if err != nil {
		return nil, err
	}
	defer zip_file_cache.Close()

	// Get the zip member from the zip file.
	return zip_file_cache.Open(full_path, self.nocase)
}

func (self *ZipFileSystemAccessor) ReadDir(
	file_path string) ([]accessors.FileInfo, error) {

	full_path, err := self.ParsePath(file_path)
	if err != nil {
		return nil, err
	}

	return self.ReadDirWithOSPath(full_path)
}

func (self *ZipFileSystemAccessor) ReadDirWithOSPath(
	full_path *accessors.OSPath) ([]accessors.FileInfo, error) {

	root, err := _GetZipFile(self, full_path)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	children, err := root.GetChildren(full_path, self.nocase)
	if err != nil {
		return nil, err
	}

	result := []accessors.FileInfo{}
	for _, item := range children {
		result = append(result, item)
	}

	return result, nil
}

// Zip files typically use standard / path separators.
func (self ZipFileSystemAccessor) ParsePath(path string) (
	*accessors.OSPath, error) {
	return accessors.NewGenericOSPath(path)
}

func (self ZipFileSystemAccessor) Describe() *accessors.AccessorDescriptor {
	return &accessors.AccessorDescriptor{
		Name:        "zip",
		Description: `Open a zip file as if it was a directory.`,

		// Doent need special permissions as we open the delegate
		Permissions: []acls.ACL_PERMISSION{},
	}
}

func (self *ZipFileSystemAccessor) New(scope vfilter.Scope) (
	accessors.FileSystemAccessor, error) {
	tag := ZipFileSystemAccessorTag
	if self.nocase {
		tag = ZipFileNoCaseSystemAccessorTag
	}

	result_any := vql_subsystem.CacheGet(scope, tag)
	if result_any == nil {
		// Create a new cache in the scope.
		result := &ZipFileSystemAccessor{
			fd_cache: make(map[string]*ZipFileCache),
			scope:    scope,
			nocase:   self.nocase,
		}
		vql_subsystem.CacheSet(scope, tag, result)

		err := vql_subsystem.GetRootScope(scope).AddDestructor(func() {
			result.CloseAll()
		})
		if err != nil {
			result.CloseAll()
			return nil, err
		}

		return result, nil
	}

	// Make a copy of the filesystem capturing the new scope.
	res := result_any.(*ZipFileSystemAccessor)
	res.Trim()

	return res.Copy(scope), nil
}
