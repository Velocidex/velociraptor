package vhdx

import (
	"io"
	"os"
	"sync"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/zip"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type VHDXFile struct {
	reader io.ReaderAt

	mu     sync.Mutex
	offset int64
	size   uint64

	closer func()
}

// Lifetime is managed by the cache
func (self *VHDXFile) Close() error {
	return nil
}

func (self *VHDXFile) Read(buff []byte) (int, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	n, err := self.reader.ReadAt(buff, self.offset)
	if err != nil {
		return 0, err
	}

	if n == 0 {
		return 0, io.EOF
	}

	self.offset += int64(n)
	return n, err
}

func (self *VHDXFile) Seek(offset int64, whence int) (int64, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if whence == os.SEEK_SET {
		self.offset = offset
	} else if whence == os.SEEK_CUR {
		self.offset += offset
	}
	return self.offset, nil
}

func (self *VHDXFile) LStat() (accessors.FileInfo, error) {
	return nil, utils.NotImplementedError
}

// Get a new copy of the handle so it can be seeked independently.
func (self *VHDXFile) _Copy() *VHDXFile {
	self.mu.Lock()
	defer self.mu.Unlock()

	return &VHDXFile{
		reader: self.reader,
		offset: 0,
		size:   self.size,
	}
}

func GetVHDXImage(full_path *accessors.OSPath, scope vfilter.Scope) (
	zip.ReaderStat, error) {

	pathspec := full_path.PathSpec()

	// The VHDX accessor must use a delegate but if one is not
	// provided we use the "auto" accessor, to open the underlying
	// file.
	if pathspec.DelegateAccessor == "" && pathspec.GetDelegatePath() == "" {
		pathspec.DelegatePath = pathspec.Path
		pathspec.DelegateAccessor = "auto"
		pathspec.Path = "/"
		full_path.SetPathSpec(pathspec)
	}

	accessor, err := accessors.GetAccessor(pathspec.DelegateAccessor, scope)
	if err != nil {
		scope.Log("vhdx: %v: did you provide a DelegateAccessor PathSpec?", err)
		return nil, err
	}

	return getCachedVHDXFile(full_path, accessor, scope)
}

func init() {
	accessors.Register("vhdx", zip.NewGzipFileSystemAccessor(
		accessors.MustNewLinuxOSPath(""), GetVHDXImage),
		`Allow reading a vhdx file.

This accessor allows access to the content of VHDX files. Note that usually
VHDX files are disk images with a partition table and an NTFS volume. You
will usually need to wrap this accessor with a suitable Offset (to account
for the parition) and parse it with the the "raw_ntfs" accessor.

For Example

    SELECT OSPath.Path AS OSPath, Size, Mode.String
    FROM glob(
       globs="*", accessor="raw_ntfs", root=pathspec(
          Path="/",
          DelegateAccessor="offset",
          DelegatePath=pathspec(
            Path="/65536",
            DelegateAccessor="vhdx",
            DelegatePath="/tmp/test.vhdx")))

`)
}
