package filesystem

import (
	"io"
	"os"
	"path"
	"strings"
	"sync/atomic"

	"github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/third_party/zip"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type MEFileSystemAccessor struct {
	*ZipFileSystemAccessor
}

// file_path refers to the fragment - we always use the current exe as
// the root.
func (self *MEFileSystemAccessor) GetZipFile(
	file_path string) (*ZipFileCache, error) {

	me, err := os.Executable()
	if err != nil {
		return nil, err
	}

	self.mu.Lock()
	zip_file_cache, pres := self.fd_cache[me]
	self.mu.Unlock()

	if !pres {
		accessor, err := glob.GetAccessor("file", self.scope)
		if err != nil {
			return nil, err
		}

		fd, err := accessor.Open(me)
		if err != nil {
			return nil, err
		}

		reader, ok := fd.(io.ReaderAt)
		if !ok {
			return nil, errors.New("file is not seekable")
		}

		stat, err := fd.Stat()
		if err != nil {
			return nil, err
		}

		zip_file, err := zip.NewReader(reader, stat.Size())
		if err != nil {
			return nil, err
		}

		zip_file_cache = &ZipFileCache{
			zip_file: zip_file,
			fd:       fd,
			refs:     1,
		}

		self.mu.Lock()
		self.fd_cache[me] = zip_file_cache

		for _, i := range zip_file.File {
			file_path := path.Clean(i.Name)
			zip_file_cache.lookup = append(zip_file_cache.lookup,
				_CDLookup{
					components:  strings.Split(file_path, "/"),
					member_file: i,
				})
		}
		self.mu.Unlock()
	}

	atomic.AddInt64(&zip_file_cache.refs, 1)

	return zip_file_cache, nil
}

func (self *MEFileSystemAccessor) Lstat(file_path string) (glob.FileInfo, error) {
	root, err := self.GetZipFile(file_path)
	if err != nil {
		return nil, err
	}

	components := utils.SplitComponents(file_path)
	return root.GetZipInfo(components, file_path)
}

func (self *MEFileSystemAccessor) Open(filename string) (glob.ReadSeekCloser, error) {
	// Fetch the zip file from cache again.
	zip_file_cache, err := self.GetZipFile(filename)
	if err != nil {
		return nil, err
	}

	// Get the zip member from the zip file.
	fd, err := zip_file_cache.Open(
		fragmentToComponents(filename), filename)
	if err != nil {
		zip_file_cache.Close()
		return nil, err
	}
	return fd, nil
}

func (self *MEFileSystemAccessor) ReadDir(file_path string) ([]glob.FileInfo, error) {
	root, err := self.GetZipFile(file_path)
	if err != nil {
		return nil, err
	}

	components := utils.SplitComponents(file_path)
	children, err := root.GetChildren(components)
	if err != nil {
		return nil, err
	}

	result := []glob.FileInfo{}
	for _, item := range children {
		item.SetFullPath(path.Join(file_path, item.Name()))
		result = append(result, item)
	}

	return result, nil
}

func (self *MEFileSystemAccessor) GetRoot(path string) (string, string, error) {
	return "/", path, nil
}

func (self MEFileSystemAccessor) PathJoin(root, stem string) string {
	return path.Join(root, stem)
}

func (self MEFileSystemAccessor) New(scope vfilter.Scope) (glob.FileSystemAccessor, error) {
	base, err := (&ZipFileSystemAccessor{}).New(scope)
	if err != nil {
		return nil, err
	}
	return &MEFileSystemAccessor{base.(*ZipFileSystemAccessor)}, nil
}

func init() {
	glob.Register("me", &MEFileSystemAccessor{})
}
