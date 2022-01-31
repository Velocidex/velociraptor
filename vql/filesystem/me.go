package filesystem

import (
	"io"
	"os"
	"path"
	"strings"

	"github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/third_party/zip"
	"www.velocidex.com/golang/vfilter"
)

type MEFileSystemAccessor struct {
	*ZipFileSystemAccessor
}

// file_path refers to the fragment - we always use the current exe as
// the root.
func (self *MEFileSystemAccessor) GetZipFile(file_path *accessors.OSPath) (
	*ZipFileCache, error) {

	me, err := os.Executable()
	if err != nil {
		return nil, err
	}

	self.mu.Lock()
	zip_file_cache, pres := self.fd_cache[me]
	self.mu.Unlock()

	if !pres {
		accessor, err := accessors.GetAccessor("file", self.scope)
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

		stat, err := accessor.Lstat(me)
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

	zip_file_cache.IncRef()

	return zip_file_cache, nil
}

func (self *MEFileSystemAccessor) Lstat(serialized_path string) (
	accessors.FileInfo, error) {
	full_path := accessors.NewPathspecOSPath(serialized_path)
	root, err := self.GetZipFile(full_path)
	if err != nil {
		return nil, err
	}

	return root.GetZipInfo(full_path)
}

func (self *MEFileSystemAccessor) Open(serialized_path string) (
	accessors.ReadSeekCloser, error) {
	// Fetch the zip file from cache again.
	full_path := accessors.NewPathspecOSPath(serialized_path)
	zip_file_cache, err := self.GetZipFile(full_path)
	if err != nil {
		return nil, err
	}

	// Get the zip member from the zip file.
	fd, err := zip_file_cache.Open(full_path)
	if err != nil {
		zip_file_cache.Close()
		return nil, err
	}
	return fd, nil
}

func (self *MEFileSystemAccessor) ReadDir(serialized_path string) (
	[]accessors.FileInfo, error) {

	full_path := accessors.NewPathspecOSPath(serialized_path)
	root, err := self.GetZipFile(full_path)
	if err != nil {
		return nil, err
	}

	children, err := root.GetChildren(full_path.Components)
	if err != nil {
		return nil, err
	}

	result := []accessors.FileInfo{}
	for _, item := range children {
		item.SetFullPath(full_path.Append(item.Name()))
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

func (self MEFileSystemAccessor) New(scope vfilter.Scope) (
	accessors.FileSystemAccessor, error) {
	base, err := (&ZipFileSystemAccessor{}).New(scope)
	if err != nil {
		return nil, err
	}
	return &MEFileSystemAccessor{base.(*ZipFileSystemAccessor)}, nil
}

func init() {
	accessors.Register("me", &MEFileSystemAccessor{},
		`Access files bundled inside the Velociraptor binary itself. This is used for unpacking extra files delivered by the Offline Collector`)
}
