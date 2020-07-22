package filesystem

import (
	"io"
	"os"
	"path"
	"strings"

	"github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/third_party/zip"
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
	defer self.mu.Unlock()

	zip_file_cache, pres := self.fd_cache[me]
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
		}

		self.fd_cache[me] = zip_file_cache

		for _, i := range zip_file.File {
			file_path := path.Clean(i.Name)
			zip_file_cache.lookup = append(zip_file_cache.lookup,
				_CDLookup{
					components: strings.Split(file_path, "/"),
					info:       i,
				})
		}
	}

	return zip_file_cache, nil
}

func (self *MEFileSystemAccessor) Lstat(file_path string) (glob.FileInfo, error) {
	root, err := self.GetZipFile(file_path)
	if err != nil {
		return nil, err
	}

	// Make it absolute.
	file_path = path.Clean(path.Join("/", file_path))

	components := []string{}
	for _, i := range strings.Split(file_path, "/") {
		if i != "" {
			components = append(components, i)
		}
	}

loop:
	for _, cd_cache := range root.lookup {
		if len(components) != len(cd_cache.components) {
			continue
		}

		for j := range components {
			if components[j] != cd_cache.components[j] {
				continue loop
			}
		}

		return &ZipFileInfo{
			info:       cd_cache.info,
			_name:      components[len(components)-1],
			_full_path: file_path,
		}, nil
	}

	return nil, errors.New("Not found.")
}

func (self *MEFileSystemAccessor) Open(path string) (glob.ReadSeekCloser, error) {
	info_generic, err := self.Lstat(path)
	if err != nil {
		return nil, err
	}

	info := info_generic.(*ZipFileInfo)

	fd, err := info.info.Open()
	if err != nil {
		return nil, err
	}

	return &SeekableZip{ReadCloser: fd, info: info}, nil
}

func (self *MEFileSystemAccessor) ReadDir(file_path string) ([]glob.FileInfo, error) {
	root, err := self.GetZipFile(file_path)
	if err != nil {
		return nil, err
	}

	file_path = path.Clean(path.Join("/", file_path))

	components := []string{}
	for _, i := range strings.Split(file_path, "/") {
		if i != "" {
			components = append(components, i)
		}
	}

	result := []glob.FileInfo{}

	// Determine if we already emitted this file. O(n) but if n is
	// small it should be faster than map.
	name_in_result := func(name string) bool {
		for _, item := range result {
			if item.Name() == name {
				return true
			}
		}
		return false
	}

loop:
	for _, cd_cache := range root.lookup {
		for j := range components {
			if components[j] != cd_cache.components[j] {
				continue loop
			}
		}

		if len(cd_cache.components) > len(components) {
			// member is either a directory or a file.
			member_name := cd_cache.components[len(components)]

			full_path := path.Join(
				cd_cache.components[:len(components)+1]...)

			member := &ZipFileInfo{
				_name:      member_name,
				_full_path: "/" + full_path,
			}

			// It is a file if the components are an exact match.
			if len(cd_cache.components) == len(components)+1 {
				member.info = cd_cache.info
			}

			if !name_in_result(member_name) {
				result = append(result, member)
			}
		}
	}

	return result, nil
}

func (self *MEFileSystemAccessor) GetRoot(path string) (string, string, error) {
	return "/", path, nil
}

func (self MEFileSystemAccessor) PathJoin(root, stem string) string {
	return path.Join(root, stem)
}

func (self MEFileSystemAccessor) New(scope *vfilter.Scope) (glob.FileSystemAccessor, error) {
	base, err := ZipFileSystemAccessor{}.New(scope)
	if err != nil {
		return nil, err
	}
	return &MEFileSystemAccessor{base.(*ZipFileSystemAccessor)}, nil
}

func init() {
	glob.Register("me", &MEFileSystemAccessor{})
}
