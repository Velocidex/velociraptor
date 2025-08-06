package fat

// This is an accessor which parses a FAT filesystem
import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"sync"
	"time"

	fat "github.com/Velocidex/go-fat/parser"
	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/vfilter"
)

const (
	// Scope cache tag for the FAT parser
	FATFileSystemTag = "_FAT"
)

type FATFileInfo struct {
	info       *fat.DirectoryEntry
	_full_path *accessors.OSPath
}

func (self *FATFileInfo) IsDir() bool {
	return self.info.IsDir
}

func (self *FATFileInfo) Size() int64 {
	return int64(self.info.Size)
}

func (self *FATFileInfo) Data() *ordereddict.Dict {
	result := ordereddict.NewDict().
		Set("first_cluster", self.info.FirstCluster).
		Set("attr", self.info.Attribute).
		Set("short_name", self.info.ShortName)

	if self.info.IsDeleted {
		result.Set("deleted", true)
	}

	return result
}

func (self *FATFileInfo) Name() string {
	return self.info.Name
}

func (self *FATFileInfo) UniqueName() string {
	return self._full_path.String()
}

func (self *FATFileInfo) Mode() os.FileMode {
	var result os.FileMode = 0755
	if self.IsDir() {
		result |= os.ModeDir
	}
	return result
}

func (self *FATFileInfo) ModTime() time.Time {
	return self.info.Mtime
}

func (self *FATFileInfo) FullPath() string {
	return self._full_path.String()
}

func (self *FATFileInfo) OSPath() *accessors.OSPath {
	return self._full_path
}

func (self *FATFileInfo) Btime() time.Time {
	return self.info.Ctime
}

func (self *FATFileInfo) Mtime() time.Time {
	return self.info.Mtime
}

func (self *FATFileInfo) Ctime() time.Time {
	return self.info.Ctime
}

func (self *FATFileInfo) Atime() time.Time {
	return self.info.Atime
}

// Not supported
func (self *FATFileInfo) IsLink() bool {
	return false
}

func (self *FATFileInfo) GetLink() (*accessors.OSPath, error) {
	return nil, errors.New("Not implemented")
}

type FATFileSystemAccessor struct {
	scope vfilter.Scope

	// The delegate accessor we use to open the underlying volume.
	accessor string
	device   *accessors.OSPath

	root *accessors.OSPath
}

func (self FATFileSystemAccessor) Describe() *accessors.AccessorDescriptor {
	return &accessors.AccessorDescriptor{
		Name:        "fat",
		Description: `Access the FAT filesystem inside an image by parsing FAT.`,
	}
}

func (self FATFileSystemAccessor) New(scope vfilter.Scope) (
	accessors.FileSystemAccessor, error) {
	// Create a new cache in the scope.
	return &FATFileSystemAccessor{
		scope:    scope,
		device:   self.device,
		accessor: self.accessor,
		root:     self.root,
	}, nil
}

func (self FATFileSystemAccessor) ParsePath(path string) (
	*accessors.OSPath, error) {
	return accessors.NewWindowsNTFSPath(path)
}

func (self *FATFileSystemAccessor) ReadDir(path string) (
	res []accessors.FileInfo, err error) {
	// Normalize the path
	fullpath, err := self.ParsePath(path)
	if err != nil {
		return nil, err
	}

	return self.ReadDirWithOSPath(fullpath)
}

func (self *FATFileSystemAccessor) ReadDirWithOSPath(
	fullpath *accessors.OSPath) (res []accessors.FileInfo, err error) {
	defer func() {
		r := recover()
		if r != nil {
			fmt.Printf("PANIC %v\n", r)
			debug.PrintStack()
			err, _ = r.(error)
		}
	}()

	result := []accessors.FileInfo{}

	fat_ctx, err := GetFatContext(self.scope, self.device, fullpath, self.accessor)
	if err != nil {
		return nil, err
	}

	// Open the device path from the root.
	dir, err := fat_ctx.ListDirectoryComponents(fullpath.Components)
	if err != nil {
		return nil, err
	}

	// List the directory.
	for _, info := range dir {
		// Skip these useless directories.
		if info.Name == "." || info.Name == ".." {
			continue
		}

		result = append(result, &FATFileInfo{
			info:       info,
			_full_path: fullpath.Append(info.Name),
		})
	}
	return result, nil
}

// Adapt a ReadSeeker onto the ReadAtter that go-ntfs provides.
type readAdapter struct {
	sync.Mutex

	info   accessors.FileInfo
	pos    int64
	reader io.ReaderAt
}

func (self *readAdapter) Read(buf []byte) (res int, err error) {
	self.Lock()
	defer self.Unlock()

	defer func() {
		r := recover()
		if r != nil {
			fmt.Printf("PANIC %v\n", r)
			debug.PrintStack()
			err, _ = r.(error)
		}
	}()

	res, err = self.reader.ReadAt(buf, self.pos)

	// If ReadAt is unable to read anything it means an EOF.
	if res == 0 {
		// The NTFS cache may be flushed during this read and in this
		// case the file handle will be closed on us during the
		// read. This usually shows up as an EOF read with 0 length.
		// See Issue
		// https://github.com/Velocidex/velociraptor/issues/2153

		// We catch this issue by issuing one more read just to make
		// sure. Usually we are wrapping a ReadAtter here and we do
		// not expect to see a EOF anyway. In the case of NTFS the
		// extra read will re-open the underlying device file with a
		// new NTFS context (reparsing the $MFT and purging all the
		// caches) so the next read will succeed.
		res, err = self.reader.ReadAt(buf, self.pos)
		if res == 0 {
			// Still EOF - give up
			return res, io.EOF
		}
	}

	self.pos += int64(res)

	return res, err
}

func (self *readAdapter) ReadAt(buf []byte, offset int64) (int, error) {
	self.Lock()
	defer self.Unlock()
	self.pos = offset

	return self.reader.ReadAt(buf, offset)
}

func (self *readAdapter) Close() error {
	return nil
}

func (self *readAdapter) Seek(offset int64, whence int) (int64, error) {
	self.Lock()
	defer self.Unlock()

	self.pos = offset
	return self.pos, nil
}

func (self *FATFileSystemAccessor) Open(
	path string) (res accessors.ReadSeekCloser, err error) {

	full_path, err := self.ParsePath(path)
	if err != nil {
		return nil, err
	}

	return self.OpenWithOSPath(full_path)
}

func (self *FATFileSystemAccessor) OpenWithOSPath(
	fullpath *accessors.OSPath) (res accessors.ReadSeekCloser, err error) {

	defer func() {
		r := recover()
		if r != nil {
			fmt.Printf("PANIC %v\n", r)
			debug.PrintStack()
			err, _ = r.(error)
		}
	}()

	fat_ctx, err := GetFatContext(self.scope, self.device, fullpath, self.accessor)
	if err != nil {
		return nil, err
	}

	// Open the device path from the root.
	stream, err := fat_ctx.OpenComponents(fullpath.Components)
	if err != nil {
		return nil, err
	}

	return &readAdapter{
		info: &FATFileInfo{
			info:       stream.Info,
			_full_path: fullpath,
		},
		reader: stream,
	}, nil
}

func (self *FATFileSystemAccessor) Lstat(
	path string) (res accessors.FileInfo, err error) {

	fullpath, err := self.ParsePath(path)
	if err != nil {
		return nil, err
	}

	return self.LstatWithOSPath(fullpath)
}

func (self *FATFileSystemAccessor) LstatWithOSPath(
	fullpath *accessors.OSPath) (res accessors.FileInfo, err error) {
	defer func() {
		r := recover()
		if r != nil {
			fmt.Printf("PANIC %v\n", r)
			debug.PrintStack()
			err, _ = r.(error)
		}
	}()

	fat_ctx, err := GetFatContext(self.scope, self.device, fullpath, self.accessor)
	if err != nil {
		return nil, err
	}

	stat, err := fat_ctx.StatComponents(fullpath.Components)
	if err != nil {
		return nil, err
	}

	return &FATFileInfo{
		info:       stat,
		_full_path: fullpath,
	}, nil
}

func init() {
	accessors.Register(&FATFileSystemAccessor{})

	json.RegisterCustomEncoder(&FATFileInfo{}, accessors.MarshalGlobFileInfo)
}
