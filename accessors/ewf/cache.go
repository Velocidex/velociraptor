package ewf

import (
	"errors"
	"io"
	"strings"
	"sync"

	"github.com/Velocidex/go-ewf/parser"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

const (
	EWF_CACHE_TAG = "__EWF_CACHE"
)

// Don't bother expiring this until the end of the query.
type ewfCache struct {
	mu sync.Mutex

	cache map[string]*EWFReader
}

func (self *ewfCache) Get(key string) (*EWFReader, bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	r, pres := self.cache[key]
	return r, pres
}

func (self *ewfCache) Set(key string, r *EWFReader) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.cache[key] = r
}

func (self *ewfCache) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	for _, r := range self.cache {
		r._ReallyClose()
	}
}

func getCachedEWFFile(
	full_path *accessors.OSPath,
	accessor accessors.FileSystemAccessor,
	scope vfilter.Scope) (*EWFReader, error) {

	cache, pres := vql_subsystem.CacheGet(scope, EWF_CACHE_TAG).(*ewfCache)
	if !pres {
		cache = &ewfCache{
			cache: make(map[string]*EWFReader),
		}
		err := vql_subsystem.GetRootScope(scope).AddDestructor(cache.Close)
		if err != nil {
			return nil, err
		}
		vql_subsystem.CacheSet(scope, EWF_CACHE_TAG, cache)
	}

	key := full_path.String()
	res, pres := cache.Get(key)
	if pres {
		// Give a copy of the cache object so it can be seeked
		// independently.
		return res.Copy(), nil
	}

	// Try to open the EWF file
	options := &parser.EWFOptions{
		LRUSize: 100,
	}

	files, err := getAllVolumes(full_path, accessor, scope)
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return nil, errors.New("No volumes found")
	}

	// Adapt all these readers for the EWF object
	files_readat := make([]io.ReaderAt, 0, len(files))
	for _, r := range files {
		files_readat = append(files_readat, utils.MakeReaderAtter(r))
	}

	ewf_volume, err := parser.OpenEWFFile(options, files_readat...)
	if err != nil {
		for _, fd := range files {
			fd.Close()
		}
		return nil, err
	}

	ewf := &EWFReader{
		readers: files,
		ewf:     ewf_volume,
	}

	cache.Set(key, ewf)
	scope.Log("ewf: Opened EWF file %v\n", key)

	return ewf, nil
}

func getAllVolumes(
	full_path *accessors.OSPath,
	accessor accessors.FileSystemAccessor,
	scope vfilter.Scope) (
	[]io.ReadSeekCloser, error) {

	result := []io.ReadSeekCloser{}

	delegate, err := full_path.Delegate(scope)
	if err != nil {
		return nil, err
	}

	basename := delegate.Basename()
	dirname := delegate.Dirname()

	if strings.HasSuffix(basename, ".E01") ||
		strings.HasSuffix(basename, ".e01") {
		prefix := basename[:len(basename)-4]

		children, err := accessor.ReadDirWithOSPath(dirname)
		if err == nil {
			// Technically a volume set can use all the letters so we
			// cant assume it has to have an .Exx extension.
			for _, c := range children {
				if !strings.HasPrefix(c.Name(), prefix) {
					continue
				}

				extension := c.Name()[len(prefix):]
				if len(extension) != 4 || extension[0] != '.' {
					continue
				}

				fd, err := accessor.OpenWithOSPath(c.OSPath())
				if err == nil {
					result = append(result, fd)
				}
				scope.Log("ewf: Found Segment file %v\n", c.OSPath())
			}
		}
	}

	if len(result) == 0 {
		fd, err := accessor.OpenWithOSPath(delegate)
		if err != nil {
			return nil, err
		}
		scope.Log("ewf: Found Segment file %v\n", delegate)
		result = append(result, fd)
	}

	return result, nil
}
