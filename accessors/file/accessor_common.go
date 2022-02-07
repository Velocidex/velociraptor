// +build linux darwin freebsd

package file

import (
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/Velocidex/ordereddict"
	errors "github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	fileAccessorCurrentOpened = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "accessor_file_current_open",
		Help: "Number of currently opened files with the file accessor.",
	})
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
	if self.IsLink() {
		path := self.FullPath()
		target, err := os.Readlink(path)
		if err == nil {
			return ordereddict.NewDict().
				Set("Link", target)
		}
	}

	result := ordereddict.NewDict()
	sys, ok := self._FileInfo.Sys().(*syscall.Stat_t)
	if ok {
		result.Set("DevMajor", (sys.Dev>>8)&0xff).
			Set("DevMinor", sys.Dev&0xFF)
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

	return self._full_path.Parse(ret), nil
}

func (self *OSFileInfo) _Sys() *syscall.Stat_t {
	return self._FileInfo.Sys().(*syscall.Stat_t)
}

// Real implementation for non windows OSs:
type OSFileSystemAccessor struct {
	context *AccessorContext

	allow_raw_access bool

	root *accessors.OSPath
}

func (self OSFileSystemAccessor) ParsePath(path string) *accessors.OSPath {
	return self.root.Parse(path)
}

func (self OSFileSystemAccessor) New(scope vfilter.Scope) (
	accessors.FileSystemAccessor, error) {

	// Check we have permission to open files.
	err := vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_READ)
	if err != nil {
		return nil, err
	}

	return &OSFileSystemAccessor{
		context: &AccessorContext{
			links: make(map[_inode]bool),
		},
		allow_raw_access: self.allow_raw_access,
		root:             self.root,
	}, nil
}

func (self OSFileSystemAccessor) Lstat(filename string) (accessors.FileInfo, error) {
	full_path := self.ParsePath(filename)
	filename = full_path.PathSpec().Path

	lstat, err := os.Lstat(filename)
	if err != nil {
		return nil, err
	}

	return &OSFileInfo{
		_FileInfo:     lstat,
		_full_path:    full_path.Copy(),
		_accessor_ctx: self.context,
	}, nil
}

func (self OSFileSystemAccessor) ReadDir(dir string) ([]accessors.FileInfo, error) {
	full_path := self.root.Parse(dir)
	dir = full_path.PathSpec().Path

	lstat, err := os.Lstat(dir)
	if err != nil {
		return nil, err
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
		target, err := os.Readlink(dir)
		if err == nil {
			lstat, err := os.Lstat(target)
			// Target of the link is not there or inaccessible or
			// points to something that is not a directory - just
			// ignore it with no errors.
			if err != nil || !lstat.IsDir() {
				return nil, nil
			}
		}
	}

	files, err := utils.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var result []accessors.FileInfo
	for _, f := range files {
		result = append(result,
			&OSFileInfo{
				_FileInfo:     f,
				_full_path:    full_path.Append(f.Name()),
				_accessor_ctx: self.context,
			})
	}

	return result, nil
}

// Wrap the os.File object to keep track of open file handles.
type OSFileWrapper struct {
	*os.File
}

func (self OSFileWrapper) Close() error {
	fileAccessorCurrentOpened.Dec()
	return self.File.Close()
}

func (self OSFileSystemAccessor) Open(path string) (accessors.ReadSeekCloser, error) {
	var err error

	// Clean the path
	full_path := self.ParsePath(path)
	path = full_path.PathSpec().Path

	// Eval any symlinks directly
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}

	// Usually we dont allow direct access to devices otherwise a
	// recursive yara scan can get into /proc/ and crash the
	// kernel. Sometimes this is exactly what we want so we provide
	// the "raw_file" accessor.
	if !self.allow_raw_access {
		lstat, err := os.Stat(path)
		if err != nil {
			return nil, err
		}

		if !lstat.Mode().IsRegular() {
			return nil, errors.New("Only regular files supported")
		}
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	fileAccessorCurrentOpened.Inc()
	return OSFileWrapper{file}, nil
}

func init() {
	json.RegisterCustomEncoder(&OSFileInfo{}, accessors.MarshalGlobFileInfo)
}
