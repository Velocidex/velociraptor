//go:build linux || darwin || freebsd
// +build linux darwin freebsd

package file

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Velocidex/ordereddict"
	errors "github.com/go-errors/errors"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

var (
	fileAccessorCurrentOpened = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "accessor_file_current_open",
		Help: "Number of currently opened files with the file accessor.",
	})

	ErrNotFound = errors.New("file not found")
)

type _inode struct {
	dev, inode uint64
}

// Keep track of symlinks we visited.
type AccessorContext struct {
	mu sync.Mutex

	links map[_inode]bool
}

func (self *AccessorContext) LinkVisited(dev, inode uint64) {
	id := _inode{dev, inode}

	self.mu.Lock()
	defer self.mu.Unlock()

	self.links[id] = true
}

func (self *AccessorContext) WasLinkVisited(dev, inode uint64) bool {
	id := _inode{dev, inode}

	self.mu.Lock()
	defer self.mu.Unlock()

	_, pres := self.links[id]
	return pres
}

type OSFileInfo struct {
	_FileInfo     os.FileInfo
	_full_path    *accessors.OSPath
	_accessor_ctx *AccessorContext
	_fstype       string
}

func NewOSFileInfo(base os.FileInfo, path *accessors.OSPath) *OSFileInfo {
	return &OSFileInfo{
		_FileInfo:  base,
		_full_path: path,
		_accessor_ctx: &AccessorContext{
			links: make(map[_inode]bool),
		},
	}
}

func (self *OSFileInfo) OSPath() *accessors.OSPath {
	return self._full_path
}

func (self *OSFileInfo) Size() int64 {
	return self._FileInfo.Size()
}

func (self *OSFileInfo) Name() string {
	return self._FileInfo.Name()
}

func (self *OSFileInfo) IsDir() bool {
	return self._FileInfo.IsDir()
}

func (self *OSFileInfo) ModTime() time.Time {
	return self._FileInfo.ModTime()
}

func (self *OSFileInfo) Mode() os.FileMode {
	return self._FileInfo.Mode()
}

func (self *OSFileInfo) Sys() interface{} {
	return self._FileInfo.Sys()
}

func (self *OSFileInfo) Dev() uint64 {
	sys, ok := self._FileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return 0
	}
	return uint64(sys.Dev)
}

func (self *OSFileInfo) Data() *ordereddict.Dict {
	result := ordereddict.NewDict()
	if self.IsLink() {
		path := self.FullPath()
		target, err := os.Readlink(path)
		if err == nil {
			result.Set("Link", target)
		}
	}

	sys, ok := self._FileInfo.Sys().(*syscall.Stat_t)
	if ok {
		major, minor := splitDevNumber(uint64(sys.Dev))
		result.Set("DevMajor", major).
			Set("DevMinor", minor)
	}

	if self._fstype != "" {
		result.Set("FSType", self._fstype)
	}

	return result
}

func (self *OSFileInfo) FullPath() string {
	return self._full_path.String()
}

func (self *OSFileInfo) IsLink() bool {
	return self.Mode()&os.ModeSymlink != 0
}

func (self *OSFileInfo) GetLink() (*accessors.OSPath, error) {
	sys, ok := self._FileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, errors.New("Symlink not supported")
	}

	if self._accessor_ctx.WasLinkVisited(uint64(sys.Dev), sys.Ino) {
		return nil, errors.New("Symlink cycle detected")
	}
	self._accessor_ctx.LinkVisited(uint64(sys.Dev), sys.Ino)

	// For now we dont support links so we dont get stuck in a
	// cycle.
	ret, err := os.Readlink(self._full_path.String())
	if err != nil {
		return nil, err
	}

	return self._full_path.Parse(ret)
}

func (self *OSFileInfo) _Sys() *syscall.Stat_t {
	return self._FileInfo.Sys().(*syscall.Stat_t)
}

// Real implementation for non windows OSs:
type OSFileSystemAccessor struct {
	context *AccessorContext

	nocase bool

	root *accessors.OSPath

	scope vfilter.Scope
}

func (self OSFileSystemAccessor) ParsePath(path string) (*accessors.OSPath, error) {
	return self.root.Parse(path)
}

func (self OSFileSystemAccessor) Describe() *accessors.AccessorDescriptor {
	return &accessors.AccessorDescriptor{
		Name:        "file",
		Description: `Access files using the operating system's API. Does not allow access to raw devices.`,
		Permissions: []acls.ACL_PERMISSION{acls.FILESYSTEM_READ},
	}
}

func (self OSFileSystemAccessor) New(scope vfilter.Scope) (
	accessors.FileSystemAccessor, error) {

	// Check we have permission to open files.
	return &OSFileSystemAccessor{
		context: &AccessorContext{
			links: make(map[_inode]bool),
		},
		root:   self.root,
		nocase: self.nocase,
		scope:  scope,
	}, nil
}

// Get the closest matching filename from the directory
func getNoCase(filename *accessors.OSPath) (*accessors.OSPath, error) {
	if len(filename.Components) == 0 {
		return nil, ErrNotFound
	}

	parent := filename.Dirname()
	dirname := parent.PathSpec().Path
	basename := filename.Basename()

	names, err := utils.ReadDirNames(dirname)
	if err != nil {
		// If we are unable to open the current directory, it may be
		// that the parent directory casing is not
		// correct. Recursively get the correct parent's casing and
		// try again.
		nocase_parent, err1 := getNoCase(parent)
		if err1 != nil {
			return nil, err
		}

		dirname := nocase_parent.PathSpec().Path
		names, err1 = utils.ReadDirNames(dirname)
		if err1 != nil {
			return nil, err
		}

		// Found the correct parent, keep going.
		parent = nocase_parent
	}

	for _, name := range names {
		if strings.EqualFold(name, basename) {
			return parent.Append(name), nil
		}
	}

	return nil, ErrNotFound
}

func (self OSFileSystemAccessor) Lstat(filename string) (accessors.FileInfo, error) {
	full_path, err := self.ParsePath(filename)
	if err != nil {
		return nil, err
	}

	return self.LstatWithOSPath(full_path)
}

// On Windows filesystems are usually case insensitive.
func (self OSFileSystemAccessor) GetCanonicalFilename(
	path *accessors.OSPath) string {
	return path.String()
}

func (self OSFileSystemAccessor) LstatWithOSPath(
	full_path *accessors.OSPath) (accessors.FileInfo, error) {

	defer Instrument("LstatWithOSPath")()

	err := CheckPrefix(full_path)
	if err != nil {
		return nil, err
	}

	filename := full_path.PathSpec().Path

	lstat, err := os.Lstat(filename)
	if err != nil {
		if !self.nocase {
			return nil, err
		}

		// Try to get a case insensitive match
		nocase_name, err1 := getNoCase(full_path)
		if err1 != nil {
			return nil, err
		}

		// Try again with the nocase filename
		filename = nocase_name.PathSpec().Path
		lstat, err1 = os.Lstat(filename)
		if err1 != nil {
			return nil, err
		}

		// From here on the filename is correct.
	}

	return &OSFileInfo{
		_FileInfo:     lstat,
		_full_path:    full_path.Copy(),
		_accessor_ctx: self.context,
	}, nil
}

func (self OSFileSystemAccessor) ReadDir(dir string) ([]accessors.FileInfo, error) {
	full_path, err := self.root.Parse(dir)
	if err != nil {
		return nil, err
	}

	return self.ReadDirWithOSPath(full_path)
}

func (self *OSFileSystemAccessor) GetUnderlyingAPIFilename(
	full_path *accessors.OSPath) (string, error) {
	return full_path.PathSpec().Path, nil
}

func (self OSFileSystemAccessor) ReadDirWithOSPath(
	full_path *accessors.OSPath) ([]accessors.FileInfo, error) {

	defer Instrument("ReadDirWithOSPath")()

	err := CheckPrefix(full_path)
	if err != nil {
		return nil, err
	}

	dir := full_path.PathSpec().Path

	lstat, err := os.Lstat(dir)
	if err != nil {
		if !self.nocase {
			return nil, err
		}

		// Try to get a case insensitive match
		nocase_name, err1 := getNoCase(full_path)
		if err1 != nil {
			return nil, err
		}
		dir = nocase_name.PathSpec().Path
		lstat, err1 = os.Lstat(dir)
		if err1 != nil {
			return nil, err
		}

		// From here below dir is the correct path casing.
	}

	// Support symlinks and directories.
	if lstat.Mode()&os.ModeSymlink == 0 {
		// Not a symlink
		if !lstat.IsDir() {
			return nil, nil
		}
	} else {
		// If it is a symlink, we need to check the target of the
		// symlink and make sure it is a directory.
		target, err := filepath.EvalSymlinks(dir)
		if err == nil {
			// The target is interpreted relative to the directory of
			// the link.
			if !strings.HasPrefix(target, "/") {
				target = full_path.Dirname().PathSpec().Path + "/" + target
			}
			lstat, err := os.Lstat(target)

			// Target of the link is not there or inaccessible or
			// points to something that is not a directory - just
			// ignore it with no errors.
			if err != nil || !lstat.IsDir() {
				return nil, nil
			}

			sys, ok := lstat.Sys().(*syscall.Stat_t)
			if ok {
				// Keep track of the links we visited.
				if self.context.WasLinkVisited(
					uint64(sys.Dev), sys.Ino) {
					return nil, errors.New("Symlink cycle detected")
				}
				self.context.LinkVisited(uint64(sys.Dev), sys.Ino)
			}
		}
		dir = target
	}

	dirfstype := getFSType(dir)

	files, err := utils.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var result []accessors.FileInfo
	for _, f := range files {
		fp := full_path.Append(f.Name())
		err := CheckPrefix(fp)
		if err != nil {
			continue
		}

		var fstype string
		if f.IsDir() {
			fstype = getFSType(fp.String())
		} else {
			fstype = dirfstype
		}
		result = append(result,
			&OSFileInfo{
				_FileInfo:     f,
				_full_path:    fp,
				_accessor_ctx: self.context,
				_fstype:       fstype,
			})
	}

	return result, nil
}

// Wrap the os.File object to keep track of open file handles.
type OSFileWrapper struct {
	*os.File
	closed bool
}

func (self *OSFileWrapper) DebugString() string {
	return fmt.Sprintf("OSFileWrapper %v (closed %v)", self.Name(), self.closed)
}

func (self *OSFileWrapper) Close() error {
	fileAccessorCurrentOpened.Dec()
	self.closed = true
	return self.File.Close()
}

func (self *OSFileSystemAccessor) Open(path string) (accessors.ReadSeekCloser, error) {
	// Clean the path
	full_path, err := self.ParsePath(path)
	if err != nil {
		return nil, err
	}

	return self.OpenWithOSPath(full_path)
}

func (self OSFileSystemAccessor) OpenWithOSPath(
	full_path *accessors.OSPath) (accessors.ReadSeekCloser, error) {

	defer Instrument("OpenWithOSPath")()

	err := CheckPrefix(full_path)
	if err != nil {
		return nil, err
	}

	path := full_path.PathSpec().Path

	// Eval any symlinks directly
	symlink_path, err := filepath.EvalSymlinks(path)
	if err != nil {
		if !self.nocase {
			return nil, err
		}

		// Try to get a case insensitive match
		nocase_name, err1 := getNoCase(full_path)
		if err1 != nil {
			return nil, err
		}

		// Try again with the nocase filename
		path = nocase_name.PathSpec().Path
		symlink_path, err1 = filepath.EvalSymlinks(path)
		if err1 != nil {
			return nil, err
		}

		// From here on path is correct.
	}

	path = symlink_path

	// Usually we dont allow direct access to devices otherwise a
	// recursive yara scan can get into /proc/ and crash the
	// kernel. Sometimes this is exactly what we want so we provide
	// the "raw_file" accessor.
	lstat, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if !lstat.Mode().IsDir() &&
		!lstat.Mode().IsRegular() {
		return nil, fmt.Errorf(
			"Only regular files supported (not %v)", path)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	fileAccessorCurrentOpened.Inc()
	return &OSFileWrapper{File: file}, nil
}

func init() {
	root_path, _ := accessors.NewLinuxOSPath("")
	accessors.Register(&OSFileSystemAccessor{
		root: root_path,
	})

	accessors.Register(accessors.DescribeAccessor(
		&OSFileSystemAccessor{
			root:   root_path,
			nocase: true,
		}, accessors.AccessorDescriptor{
			Name:        "file_nocase",
			Description: `Access files using the operating system's API. This is case insensitive - even on Unix Operating systems.`,
			Permissions: []acls.ACL_PERMISSION{acls.FILESYSTEM_READ},
		}))

	// On Linux the auto accessor is the same as file.
	accessors.Register(accessors.DescribeAccessor(
		&OSFileSystemAccessor{
			root: root_path,
		}, accessors.AccessorDescriptor{
			Name:        "auto",
			Description: `Access the file using the best accessor possible. On windows we fall back to NTFS parsing in case the file is locked or unreadable.`,
			Permissions: []acls.ACL_PERMISSION{acls.FILESYSTEM_READ},
		}))

	json.RegisterCustomEncoder(&OSFileInfo{}, accessors.MarshalGlobFileInfo)
}
