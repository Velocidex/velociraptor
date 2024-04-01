//go:build !windows
// +build !windows

package fuse

import (
	"context"
	"fmt"
	"io"
	"os"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"www.velocidex.com/golang/velociraptor/accessors"
)

// Build the directory tree
type FileNode struct {
	fs.Inode

	accessor accessors.FileSystemAccessor
	ospath   *accessors.OSPath
}

func (self FileNode) Getattr(ctx context.Context,
	f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {

	stat, err := self.accessor.LstatWithOSPath(self.ospath)
	if err != nil {
		return syscall.EIO
	}

	out.Mode = uint32(0644)
	out.Nlink = 1
	out.Mtime = uint64(stat.Mtime().Unix())
	out.Atime = out.Mtime
	out.Ctime = out.Mtime
	out.Size = uint64(stat.Size())

	const bs = 512
	out.Blksize = bs
	out.Blocks = (out.Size + bs - 1) / bs

	return 0
}

func (self *FileNode) Open(
	ctx context.Context, flags uint32) (
	fs.FileHandle, uint32, syscall.Errno) {

	// We don't return a filehandle since we don't really need
	// one.  The file content is immutable, so hint the kernel to
	// cache the data.
	return nil, fuse.FOPEN_KEEP_CACHE, 0
}

func (self *FileNode) Read(ctx context.Context,
	f fs.FileHandle, dest []byte, off int64) (
	fuse.ReadResult, syscall.Errno) {

	fd, err := self.accessor.OpenWithOSPath(self.ospath)
	if err != nil {
		return nil, syscall.EIO
	}
	defer fd.Close()

	_, err = fd.Seek(off, os.SEEK_SET)
	if err != nil {
		return nil, syscall.EIO
	}

	n, err := fd.Read(dest)
	if err != nil && err != io.EOF {
		fmt.Printf("ERROR: While opening %v: %v\n",
			self.ospath.String(), err)
		return nil, syscall.EIO
	}

	return fuse.ReadResultData(dest[:n]), 0
}

var _ = (fs.NodeOpener)((*FileNode)(nil))
var _ = (fs.NodeGetattrer)((*FileNode)(nil))
