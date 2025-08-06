package zip

import (
	"io"

	"github.com/go-errors/errors"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/config"
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

	me := config.EmbeddedFile

	mu.Lock()
	zip_file_cache, pres := self.fd_cache[me]
	mu.Unlock()

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

		mu.Lock()
		self.fd_cache[me] = zip_file_cache

		for _, i := range zip_file.File {
			full_path, err := accessors.NewLinuxOSPath(i.Name)
			if err != nil {
				continue
			}

			zip_file_cache.lookup = append(zip_file_cache.lookup,
				_CDLookup{
					full_path:   full_path,
					member_file: i,
				})
		}
		mu.Unlock()
	}

	zip_file_cache.IncRef()

	return zip_file_cache, nil
}

func (self *MEFileSystemAccessor) Lstat(serialized_path string) (
	accessors.FileInfo, error) {
	full_path, err := self.ParsePath(serialized_path)
	if err != nil {
		return nil, err
	}

	return self.LstatWithOSPath(full_path)
}

func (self *MEFileSystemAccessor) LstatWithOSPath(
	full_path *accessors.OSPath) (accessors.FileInfo, error) {

	root, err := self.GetZipFile(full_path)
	if err != nil {
		return nil, err
	}

	return root.GetZipInfo(full_path, false)
}

func (self *MEFileSystemAccessor) Open(serialized_path string) (
	accessors.ReadSeekCloser, error) {
	// Fetch the zip file from cache again.
	full_path, err := self.ParsePath(serialized_path)
	if err != nil {
		return nil, err
	}

	return self.OpenWithOSPath(full_path)
}

func (self *MEFileSystemAccessor) OpenWithOSPath(
	full_path *accessors.OSPath) (accessors.ReadSeekCloser, error) {

	zip_file_cache, err := self.GetZipFile(full_path)
	if err != nil {
		return nil, err
	}

	// Get the zip member from the zip file.
	fd, err := zip_file_cache.Open(full_path, false)
	if err != nil {
		zip_file_cache.Close()
		return nil, err
	}
	return fd, nil
}

func (self *MEFileSystemAccessor) ReadDir(file_path string) (
	[]accessors.FileInfo, error) {

	full_path, err := self.ParsePath(file_path)
	if err != nil {
		return nil, err
	}

	return self.ReadDirWithOSPath(full_path)
}

func (self *MEFileSystemAccessor) ReadDirWithOSPath(
	full_path *accessors.OSPath) ([]accessors.FileInfo, error) {

	root, err := self.GetZipFile(full_path)
	if err != nil {
		return nil, err
	}

	children, err := root.GetChildren(full_path, false)
	if err != nil {
		return nil, err
	}

	result := []accessors.FileInfo{}
	for _, item := range children {
		result = append(result, item)
	}

	return result, nil
}

func (self MEFileSystemAccessor) ParsePath(path string) (
	*accessors.OSPath, error) {
	return accessors.NewLinuxOSPath(path)
}

func (self MEFileSystemAccessor) New(scope vfilter.Scope) (
	accessors.FileSystemAccessor, error) {
	base, err := (&ZipFileSystemAccessor{}).New(scope)
	if err != nil {
		return nil, err
	}
	return &MEFileSystemAccessor{base.(*ZipFileSystemAccessor)}, nil
}

func (self MEFileSystemAccessor) Describe() *accessors.AccessorDescriptor {
	return &accessors.AccessorDescriptor{
		Name:        "me",
		Description: `Access files bundled inside the Velociraptor binary itself. This is used for unpacking extra files delivered by the Offline Collector`,
	}
}

func init() {
	accessors.Register(&MEFileSystemAccessor{})

}
