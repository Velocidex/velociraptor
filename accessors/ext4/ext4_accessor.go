package ext4

// This is an accessor which parses a Ext4 filesystem
import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"runtime/debug"
	"sync"

	ext4 "github.com/Velocidex/go-ext4/parser"
	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/vfilter"
)

type Ext4FileInfo struct {
	*ext4.FileInfo
	_full_path *accessors.OSPath
}

func (self *Ext4FileInfo) IsDir() bool {
	return self.Mode().IsDir()
}

func (self *Ext4FileInfo) Data() *ordereddict.Dict {
	data := ordereddict.NewDict().
		Set("Inode", self.Inode()).
		Set("Uid", self.Uid()).
		Set("Gid", self.Gid())

	flags := self.Flags()
	if len(flags) > 0 {
		data.Set("Flags", flags)
	}

	return data
}

func (self *Ext4FileInfo) UniqueName() string {
	return self._full_path.String()
}

func (self *Ext4FileInfo) FullPath() string {
	return self._full_path.String()
}

func (self *Ext4FileInfo) OSPath() *accessors.OSPath {
	return self._full_path
}

// Not supported
func (self *Ext4FileInfo) IsLink() bool {
	return self.Mode()&fs.ModeSymlink > 0
}

func (self *Ext4FileInfo) GetLink() (*accessors.OSPath, error) {
	return nil, errors.New("Not implemented")
}

type Ext4FileSystemAccessor struct {
	scope vfilter.Scope

	// The delegate accessor we use to open the underlying volume.
	accessor string
	device   *accessors.OSPath

	root *accessors.OSPath
}

func NewExt4FileSystemAccessor(
	scope vfilter.Scope,
	root_path *accessors.OSPath,
	device *accessors.OSPath, accessor string) *Ext4FileSystemAccessor {
	return &Ext4FileSystemAccessor{
		scope:    scope,
		accessor: accessor,
		device:   device,
		root:     root_path,
	}
}

func (self Ext4FileSystemAccessor) Describe() *accessors.AccessorDescriptor {
	return &accessors.AccessorDescriptor{
		Name:        "raw_ext4",
		Description: `Access the Ext4 filesystem inside an image by parsing the image.`,
	}
}

func (self Ext4FileSystemAccessor) New(scope vfilter.Scope) (
	accessors.FileSystemAccessor, error) {
	// Create a new cache in the scope.
	return &Ext4FileSystemAccessor{
		scope:    scope,
		device:   self.device,
		accessor: self.accessor,
		root:     self.root,
	}, nil
}

func (self Ext4FileSystemAccessor) ParsePath(path string) (
	*accessors.OSPath, error) {
	return accessors.NewLinuxOSPath(path)
}

func (self *Ext4FileSystemAccessor) ReadDir(path string) (
	res []accessors.FileInfo, err error) {
	// Normalize the path
	fullpath, err := self.ParsePath(path)
	if err != nil {
		return nil, err
	}

	return self.ReadDirWithOSPath(fullpath)
}

func (self *Ext4FileSystemAccessor) ReadDirWithOSPath(
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

	ext4_ctx, err := GetExt4Context(self.scope, self.device, fullpath, self.accessor)
	if err != nil {
		return nil, err
	}

	// Open the device path from the root.
	inode, err := ext4_ctx.OpenInodeWithPath(fullpath.Components)
	if err != nil {
		return nil, err
	}

	dir, err := inode.Dir(ext4_ctx)
	if err != nil {
		return nil, err
	}

	// List the directory.
	for _, info := range dir {
		name := info.Name()

		// Skip these useless directories.
		if name == "" || name == "." || name == ".." {
			continue
		}

		result = append(result, &Ext4FileInfo{
			FileInfo:   info,
			_full_path: fullpath.Append(info.Name()),
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

func (self *Ext4FileSystemAccessor) Open(
	path string) (res accessors.ReadSeekCloser, err error) {

	full_path, err := self.ParsePath(path)
	if err != nil {
		return nil, err
	}

	return self.OpenWithOSPath(full_path)
}

func (self *Ext4FileSystemAccessor) OpenWithOSPath(
	fullpath *accessors.OSPath) (res accessors.ReadSeekCloser, err error) {

	defer func() {
		r := recover()
		if r != nil {
			fmt.Printf("PANIC %v\n", r)
			debug.PrintStack()
			err, _ = r.(error)
		}
	}()

	ext4_ctx, err := GetExt4Context(self.scope, self.device, fullpath, self.accessor)
	if err != nil {
		return nil, err
	}

	// Open the device path from the root.
	inode, err := ext4_ctx.OpenInodeWithPath(fullpath.Components)
	if err != nil {
		return nil, err
	}

	stream, err := inode.GetReader(ext4_ctx)
	if err != nil {
		return nil, err
	}

	return &readAdapter{
		info: &Ext4FileInfo{
			FileInfo:   inode.Stat(),
			_full_path: fullpath,
		},
		reader: stream,
	}, nil
}

func (self *Ext4FileSystemAccessor) Lstat(
	path string) (res accessors.FileInfo, err error) {

	fullpath, err := self.ParsePath(path)
	if err != nil {
		return nil, err
	}

	return self.LstatWithOSPath(fullpath)
}

func (self *Ext4FileSystemAccessor) LstatWithOSPath(
	fullpath *accessors.OSPath) (res accessors.FileInfo, err error) {
	defer func() {
		r := recover()
		if r != nil {
			fmt.Printf("PANIC %v\n", r)
			debug.PrintStack()
			err, _ = r.(error)
		}
	}()

	ext4_ctx, err := GetExt4Context(self.scope, self.device, fullpath, self.accessor)
	if err != nil {
		return nil, err
	}

	inode, err := ext4_ctx.OpenInodeWithPath(fullpath.Components)
	if err != nil {
		return nil, err
	}

	stat := inode.Stat()
	return &Ext4FileInfo{
		FileInfo:   stat,
		_full_path: fullpath,
	}, nil
}

func init() {
	accessors.Register(&Ext4FileSystemAccessor{})

	json.RegisterCustomEncoder(&Ext4FileInfo{}, accessors.MarshalGlobFileInfo)
}
