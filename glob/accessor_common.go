//go:build linux || darwin || freebsd
// +build linux darwin freebsd

package glob

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Velocidex/ordereddict"
	errors "github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
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
	_full_path    string
	_accessor_ctx *AccessorContext
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

func (self *OSFileInfo) Data() interface{} {
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
	return self._full_path
}

func (self *OSFileInfo) IsLink() bool {
	return self.Mode()&os.ModeSymlink != 0
}

func (self *OSFileInfo) GetLink() (string, error) {
	sys, ok := self._FileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return "", errors.New("Symlink not supported")
	}

	if self._accessor_ctx.WasLinkVisited(uint64(sys.Dev), sys.Ino) {
		return "", errors.New("Symlink cycle detected")
	}
	self._accessor_ctx.LinkVisited(uint64(sys.Dev), sys.Ino)

	// For now we dont support links so we dont get stuck in a
	// cycle.
	ret, err := os.Readlink(strings.TrimRight(self._full_path, "/"))
	if err != nil {
		return "", err
	}

	if !strings.HasPrefix(ret, "/") {
		ret = "/" + ret
	}

	return ret, nil
}

func (self *OSFileInfo) _Sys() *syscall.Stat_t {
	return self._FileInfo.Sys().(*syscall.Stat_t)
}

// Real implementation for non windows OSs:
type OSFileSystemAccessor struct {
	context *AccessorContext

	allow_raw_access bool
	root             string
}

func (self OSFileSystemAccessor) New(scope vfilter.Scope) (FileSystemAccessor, error) {
	return &OSFileSystemAccessor{
		context: &AccessorContext{
			links: make(map[_inode]bool),
		},
		allow_raw_access: self.allow_raw_access,
	}, nil
}

func (self OSFileSystemAccessor) Lstat(filename string) (FileInfo, error) {
	pathSpec, err := PathSpecFromString(filename)
	if err != nil {
		return nil, err
	}

	lstat, err := os.Lstat(path.Join(pathSpec.DelegatePath, pathSpec.Path))
	if err != nil {
		return nil, err
	}

	return &OSFileInfo{
		_FileInfo:     lstat,
		_full_path:    pathSpec.String(),
		_accessor_ctx: self.context,
	}, nil
}

func (self OSFileSystemAccessor) ReadDir(dir string) ([]FileInfo, error) {
	pathSpec, err := PathSpecFromString(dir)
	if err != nil {
		return nil, err
	}

	fullpath := path.Join(pathSpec.DelegatePath, pathSpec.Path)

	lstat, err := os.Lstat(fullpath)
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
		target, err := os.Readlink(fullpath)
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

	files, err := utils.ReadDir(fullpath)
	if err != nil {
		return nil, err
	}

	var result []FileInfo
	for _, f := range files {
		child_pathSpec := *pathSpec
		child_pathSpec.Path = filepath.Join(child_pathSpec.Path, f.Name())
		result = append(result,
			&OSFileInfo{
				_FileInfo:     f,
				_full_path:    child_pathSpec.String(),
				_accessor_ctx: self.context,
			})
	}

	return result, nil
}

func (self *OSFileSystemAccessor) SetDataSource(dataSource string) {
	self.root = dataSource
}

// Wrap the os.File object to keep track of open file handles.
type OSFileWrapper struct {
	*os.File
}

func (self OSFileWrapper) Close() error {
	fileAccessorCurrentOpened.Dec()
	return self.File.Close()
}

func (self OSFileSystemAccessor) Open(path string) (ReadSeekCloser, error) {
	var err error

	pathSpec, err := PathSpecFromString(path)
	if err != nil {
		return nil, err
	}

	// Eval any symlinks directly
	path, err = filepath.EvalSymlinks(filepath.Join(pathSpec.Path, path))
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

var OSFileSystemAccessor_re = regexp.MustCompile(`[\\/]`)

func (self OSFileSystemAccessor) PathSplit(path string) []string {
	return OSFileSystemAccessor_re.Split(path, -1)
}

func (self OSFileSystemAccessor) PathJoin(root, stem string) string {
	pathSpec, err := PathSpecFromString(root)
	if err != nil {
		return path.Join(root, stem)
	}

	pathSpec.Path = path.Join(pathSpec.Path, strings.TrimLeft(stem, "\\/"))

	return pathSpec.String()
}

func (self *OSFileSystemAccessor) GetRoot(path string) (string, string, error) {
	return "/", path, nil
}

func init() {
	Register("file", &OSFileSystemAccessor{}, `Access files using the operating system's API. Does not allow access to raw devices.`)

	Register("raw_file", &OSFileSystemAccessor{
		allow_raw_access: true,
	}, `Access files using the operating system's API. Also allow access to raw devices.`)

	// On Linux the auto accessor is the same as file.
	Register("auto", &OSFileSystemAccessor{}, `Access the file using the best accessor possible. On windows we fall back to NTFS parsing in case the file is locked or unreadable.`)

	json.RegisterCustomEncoder(&OSFileInfo{}, MarshalGlobFileInfo)
}
